package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.bug.st/serial"
)

func main() {
	var (
		flagConfig    = flag.String("config", "config.yaml", "percorso del file di configurazione")
		flagPort      = flag.String("port", "", "porta seriale (sovrascrive config)")
		flagDelay     = flag.Float64("start-delay", 0, "ritardo avvio in secondi (0 = usa config)")
		flagDashboard = flag.Bool("dashboard", false, "mostra dashboard live nel terminale")
		flagNoColor   = flag.Bool("no-color", false, "disabilita colori ANSI nel dashboard")
		flagAttempts  = flag.Int("attempts", 0, "tentativi di unlock (0 = usa config)")
		flagWindow    = flag.Float64("window", 0, "finestra unlock in secondi (0 = usa config)")
		flagFanRPM    = flag.Int("fan-max-rpm", 0, "RPM massimo ventola NVIDIA (0 = usa config)")
		flagFanPref   = flag.String("fan-prefer", "", "sorgente ventola: auto|hwmon|nvidia")
	)
	flag.Parse()

	cfg, err := loadConfig(*flagConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Errore config: %v\n", err)
		os.Exit(1)
	}

	// Applica override da CLI
	if *flagPort != "" {
		cfg.Serial.Port = *flagPort
	}
	if *flagDelay > 0 {
		cfg.Serial.StartDelay = *flagDelay
	}
	if *flagDashboard {
		cfg.Dashboard.Enabled = true
	}
	if *flagNoColor {
		cfg.Dashboard.NoColor = true
		noColor = true
	}
	if *flagAttempts > 0 {
		cfg.Unlock.Attempts = *flagAttempts
	}
	if *flagWindow > 0 {
		cfg.Unlock.WindowSeconds = *flagWindow
	}
	if *flagFanRPM > 0 {
		cfg.Fan.MaxRPM = *flagFanRPM
	}
	if *flagFanPref != "" {
		cfg.Fan.Prefer = *flagFanPref
	}
	noColor = cfg.Dashboard.NoColor

	// Costruisci rotazione tile
	rotation := buildTileRotation(cfg)
	if len(rotation) == 0 {
		fmt.Fprintln(os.Stderr, "Nessun tile configurato — controlla 'tiles' in config.yaml")
		os.Exit(1)
	}

	// Signal handling per shutdown pulito
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Println("\n[AtomMan] Chiusura in corso…")
		cancel()
	}()

	fmt.Printf("[AtomMan] Porta: %s @ %d baud (DSRDTR=%v, start_delay=%.1fs)\n",
		cfg.Serial.Port, cfg.Serial.Baud, cfg.Serial.DSRDTR, cfg.Serial.StartDelay)
	fmt.Printf("[AtomMan] Tile in rotazione: ")
	for i, td := range rotation {
		if i > 0 {
			fmt.Print(" → ")
		}
		fmt.Print(td.Name)
	}
	fmt.Println()

	// Apertura porta seriale (include start_delay)
	port, err := openSerial(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[AtomMan] Apertura seriale fallita: %v\n", err)
		os.Exit(1)
	}
	defer port.Close()

	// Sequenza di unlock
	activated := unlockSequence(ctx, port, cfg)
	if ctx.Err() != nil {
		return
	}
	if !activated {
		fmt.Println("[WARN] Display non confermato attivo; continuo comunque.")
	} else {
		fmt.Println("[OK] Display attivo — passaggio a steady state.")
	}

	// Loop principale
	refresh := time.Duration(cfg.Dashboard.RefreshMillis) * time.Millisecond
	if refresh <= 0 {
		refresh = 500 * time.Millisecond
	}
	var (
		idx        = 0
		lastRender time.Time
	)

	for ctx.Err() == nil {
		_, ok := readENQ(port) // in steady-state usiamo seqFor fisso, non il SEQ ricevuto
		if !ok {
			if cfg.Dashboard.Enabled && time.Since(lastRender) >= refresh {
				renderDashboard(cfg)
				lastRender = time.Now()
			}
			continue
		}

		tile := rotation[idx%len(rotation)]
		payload := tile.PayloadFn(cfg)
		if err := sendTile(port, tile.ID, payload, seqFor[tile.ID]); err != nil {
			fmt.Printf("[WARN] Invio tile %s: %v\n", tile.Name, err)
		}

		if cfg.Dashboard.Enabled && time.Since(lastRender) >= refresh {
			renderDashboard(cfg)
			lastRender = time.Now()
		}

		idx++
	}
}

// unlockSequence gestisce la fase di avvio: invia CPU/GPU/MEM in loop
// echeggiando il SEQ ricevuto, finché il display non si attiva.
func unlockSequence(ctx context.Context, port serial.Port, cfg *Config) bool {
	unlockRot := []tileDesc{
		{ID: TileCPU, Name: "cpu", PayloadFn: payloadCPU},
		{ID: TileGPU, Name: "gpu", PayloadFn: payloadGPU},
		{ID: TileMEM, Name: "memory", PayloadFn: payloadMem},
	}

	for attempt := 1; attempt <= cfg.Unlock.Attempts; attempt++ {
		if ctx.Err() != nil {
			return false
		}
		fmt.Printf("[Tentativo %d/%d] Finestra unlock %.0fs — CPU→GPU→MEM\n",
			attempt, cfg.Unlock.Attempts, cfg.Unlock.WindowSeconds)

		if runUnlockAttempt(ctx, port, cfg, unlockRot) {
			fmt.Printf("[Tentativo %d] Display attivato.\n", attempt)
			return true
		}
		fmt.Printf("[Tentativo %d] Nessuna attivazione nella finestra.\n", attempt)

		if attempt < cfg.Unlock.Attempts {
			// Toggle DTR per reset del display
			port.SetDTR(false)
			time.Sleep(50 * time.Millisecond)
			port.SetDTR(true)
			time.Sleep(300 * time.Millisecond)
		}
	}
	return false
}

func runUnlockAttempt(ctx context.Context, port serial.Port, cfg *Config, unlockRot []tileDesc) bool {
	deadline := time.Now().Add(time.Duration(float64(time.Second) * cfg.Unlock.WindowSeconds))
	var (
		idx         = 0
		bootReplies = 0
		enqTimes    []time.Time
	)

	for time.Now().Before(deadline) && ctx.Err() == nil {
		seq, ok := readENQ(port)
		if !ok {
			continue
		}

		// Traccia gli ENQ degli ultimi 2 secondi
		now := time.Now()
		enqTimes = append(enqTimes, now)
		cutoff := now.Add(-2 * time.Second)
		i := 0
		for i < len(enqTimes) && enqTimes[i].Before(cutoff) {
			i++
		}
		enqTimes = enqTimes[i:]

		// Risponde con il SEQ ricevuto (echo) durante l'unlock
		tile := unlockRot[idx%len(unlockRot)]
		payload := tile.PayloadFn(cfg)
		frame := buildReply(tile.ID, seq, payload)
		port.Write(frame)
		port.Drain()
		time.Sleep(6 * time.Millisecond)

		if isBootSeq(seq) {
			bootReplies++
		}
		// Attivazione: almeno 3 risposte boot E 5 ENQ nell'ultima finestra di 2s
		if bootReplies >= 3 && len(enqTimes) >= 5 {
			return true
		}
		idx++
	}
	return false
}
