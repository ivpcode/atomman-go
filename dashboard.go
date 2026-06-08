package main

import (
	"fmt"
	"strings"
	"time"
)

// Colori ANSI
const (
	cReset  = "\033[0m"
	cRed    = "\033[91m"
	cGreen  = "\033[92m"
	cYellow = "\033[93m"
	cBlue   = "\033[94m"
	cCyan   = "\033[96m"
	cWhite  = "\033[97m"
	cDim    = "\033[2m"
)

var noColor bool

func col(s, c string) string {
	if noColor {
		return s
	}
	return c + s + cReset
}

func tempColor(temp int) string {
	switch {
	case temp < 60:
		return cGreen
	case temp < 80:
		return cYellow
	default:
		return cRed
	}
}

func usageColor(pct int) string {
	switch {
	case pct < 40:
		return cGreen
	case pct < 80:
		return cYellow
	default:
		return cRed
	}
}

// renderDashboard stampa a terminale le metriche in tempo reale.
// Pulisce lo schermo e ridisegna tutto ogni refresh.
func renderDashboard(cfg *Config) {
	// Clear + home
	fmt.Print("\033[2J\033[H")

	now := time.Now()
	fmt.Printf("%s   %s\n",
		col("AtomMan — Live Monitor", cWhite),
		col(now.Format("2006-01-02 15:04:05"), cCyan),
	)
	fmt.Println(strings.Repeat("─", 72))

	// ── CPU ─────────────────────────────────────────────────────────────────
	cpuName := cfg.CustomLabels.CPU
	if cpuName == "" {
		cpuName = cpuModel()
	}
	cTemp := cpuTempC()
	cUsage := cpuUsagePct()
	cFreq := cpuFreqKHz()

	fmt.Printf("%-18s %s\n", col("CPU:", cBlue), cpuName)
	fmt.Printf("  %-16s %s\n", "Temperatura:",
		col(fmt.Sprintf("%d °C", cTemp), tempColor(cTemp)))
	fmt.Printf("  %-16s %s\n", "Utilizzo:",
		col(fmt.Sprintf("%d %%", cUsage), usageColor(cUsage)))
	fmt.Printf("  %-16s %s\n", "Frequenza:",
		col(fmt.Sprintf("%d kHz", cFreq), cDim))
	fmt.Println()

	// ── GPU ─────────────────────────────────────────────────────────────────
	gpuName := cfg.CustomLabels.GPU
	g := getGPUInfo()
	if gpuName == "" {
		gpuName = g.Name
	}
	fmt.Printf("%-18s %s\n", col("GPU:", cBlue), gpuName)
	if g.Temp > 0 {
		fmt.Printf("  %-16s %s\n", "Temperatura:",
			col(fmt.Sprintf("%d °C", g.Temp), tempColor(g.Temp)))
	} else {
		fmt.Printf("  %-16s %s\n", "Temperatura:", col("N/D", cDim))
	}
	fmt.Printf("  %-16s %s\n", "Utilizzo:",
		col(fmt.Sprintf("%d %%", g.Util), usageColor(g.Util)))
	fmt.Println()

	// ── Memoria ─────────────────────────────────────────────────────────────
	ramName := cfg.CustomLabels.Memory
	if ramName == "" {
		ramName = getRamLabel()
	}
	m := getMemInfo()
	fmt.Printf("%-18s %s\n", col("RAM:", cBlue), ramName)
	fmt.Printf("  %-16s %s / %.1f GB totali\n", "Usata:",
		col(fmt.Sprintf("%.1f GB", m.Used), usageColor(m.UsagePct)),
		m.Total)
	fmt.Printf("  %-16s %s\n", "Utilizzo:",
		col(fmt.Sprintf("%d %%", m.UsagePct), usageColor(m.UsagePct)))
	fmt.Println()

	// ── Disco ───────────────────────────────────────────────────────────────
	diskName := cfg.CustomLabels.Disk
	if diskName == "" {
		diskName = getDiskLabel()
	}
	d := getDiskInfo()
	dTemp := getDiskTempC()
	fmt.Printf("%-18s %s\n", col("Disco:", cBlue), diskName)
	fmt.Printf("  %-16s %s\n", "Temperatura:",
		col(fmt.Sprintf("%d °C", dTemp), tempColor(dTemp)))
	fmt.Printf("  %-16s %s / %d GB totali\n", "Usato:",
		col(fmt.Sprintf("%d GB", d.Used), usageColor(d.UsagePct)),
		d.Total)
	fmt.Printf("  %-16s %s\n", "Utilizzo:",
		col(fmt.Sprintf("%d %%", d.UsagePct), usageColor(d.UsagePct)))
	fmt.Println()

	// ── Rete ────────────────────────────────────────────────────────────────
	rxKBs, txKBs, iface := globalNet.Rates(cfg.Network.Interface)
	rpm := getFanRPM(cfg.Fan.Prefer, cfg.Fan.MaxRPM)
	fmt.Printf("%-18s %s\n", col("Rete:", cBlue), col(iface, cDim))
	fmt.Printf("  %-16s %s  ↓  %s\n", "RX / TX:",
		col(fmtRate(rxKBs), cGreen), col(fmtRate(txKBs), cYellow))
	rpmStr := fmt.Sprintf("%d RPM", rpm)
	if rpm < 0 {
		rpmStr = "N/D"
	}
	fmt.Printf("  %-16s %s\n", "Ventola:", col(rpmStr, cDim))
	fmt.Println()

	// ── Volume / Batteria ───────────────────────────────────────────────────
	vol := getVolume()
	volStr := "N/D"
	if vol >= 0 {
		volStr = fmt.Sprintf("%d %%", vol)
	}
	bat := getBattery()
	batStr := "Desktop (n/a)"
	if bat != 177 {
		batStr = fmt.Sprintf("%d %%", bat)
	}
	fmt.Printf("%-18s %s\n", col("Volume:", cBlue), volStr)
	fmt.Printf("%-18s %s\n", col("Batteria:", cBlue), batStr)
	fmt.Println()

	// ── Meteo ───────────────────────────────────────────────────────────────
	w := GetWeather(cfg)
	if w != nil {
		unit := "°C"
		if cfg.Weather.Units == "imperial" {
			unit = "°F"
		}
		fmt.Printf("%-18s %s (min %d%s / max %d%s)\n",
			col("Meteo:", cBlue),
			col(w.Desc, cCyan),
			w.Lo, unit, w.Hi, unit)
		fmt.Printf("  %-16s %s\n", "Zona:", w.Zone)
	} else {
		fmt.Printf("%-18s %s\n", col("Meteo:", cBlue), col("(nessuna API key)", cDim))
	}

	fmt.Println(strings.Repeat("─", 72))
	fmt.Printf("%s  Tile attivi: %s\n",
		col("Config:", cDim),
		col(strings.Join(cfg.Tiles, " → "), cDim))
}
