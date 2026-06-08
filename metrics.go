package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ─── Utilità ────────────────────────────────────────────────────────────────

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func isDigitStr(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func runCmd(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ─── CPU ────────────────────────────────────────────────────────────────────

var (
	cpuModelOnce  sync.Once
	cpuModelCache string
)

func cpuModel() string {
	cpuModelOnce.Do(func() {
		f, err := os.Open("/proc/cpuinfo")
		if err != nil {
			cpuModelCache = "Linux CPU"
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "model name") {
				if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
					cpuModelCache = strings.TrimSpace(parts[1])
					return
				}
			}
		}
		cpuModelCache = "Linux CPU"
	})
	return cpuModelCache
}

type cpuSample struct {
	idle, total uint64
	ts          time.Time
	pct         int
}

var (
	cpuMu   sync.Mutex
	cpuPrev cpuSample
)

func cpuUsagePct() int {
	cpuMu.Lock()
	defer cpuMu.Unlock()
	if !cpuPrev.ts.IsZero() && time.Since(cpuPrev.ts) < 500*time.Millisecond {
		return cpuPrev.pct
	}

	line := ""
	f, err := os.Open("/proc/stat")
	if err == nil {
		sc := bufio.NewScanner(f)
		if sc.Scan() {
			line = sc.Text()
		}
		f.Close()
	}
	if line == "" {
		return 0
	}

	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0
	}

	var vals []uint64
	for _, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		vals = append(vals, n)
	}

	idle := vals[3]
	if len(vals) > 4 {
		idle += vals[4] // iowait
	}
	total := uint64(0)
	for _, v := range vals {
		total += v
	}

	pct := 0
	if !cpuPrev.ts.IsZero() && total > cpuPrev.total {
		dIdle := idle - cpuPrev.idle
		dTotal := total - cpuPrev.total
		if dTotal > 0 {
			pct = int(math.Round(100 * float64(dTotal-dIdle) / float64(dTotal)))
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
		}
	}
	cpuPrev = cpuSample{idle: idle, total: total, ts: time.Now(), pct: pct}
	return pct
}

func cpuFreqKHz() int {
	maxFreq := 0
	dirs, _ := filepath.Glob("/sys/devices/system/cpu/cpu*/cpufreq")
	for _, dir := range dirs {
		for _, suffix := range []string{"scaling_cur_freq", "cpuinfo_cur_freq"} {
			s := strings.TrimSpace(readFile(filepath.Join(dir, suffix)))
			if v, err := strconv.Atoi(s); err == nil && v > maxFreq {
				maxFreq = v
				break
			}
		}
	}
	if maxFreq > 0 {
		return maxFreq
	}
	// Fallback: /proc/cpuinfo MHz → kHz
	re := regexp.MustCompile(`cpu MHz\s*:\s*([\d.]+)`)
	max := 0.0
	for _, m := range re.FindAllStringSubmatch(readFile("/proc/cpuinfo"), -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > max {
			max = v
		}
	}
	return int(max * 1000)
}

var (
	cpuTempOnce sync.Once
	cpuTempPath string
)

func findCPUTempPath() string {
	cpuTempOnce.Do(func() {
		preferred := map[string]bool{
			"coretemp": true, "k10temp": true, "zenpower": true,
			"cpu_thermal": true, "acpitz": true,
		}
		hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
		for _, hw := range hwmons {
			name := strings.ToLower(strings.TrimSpace(readFile(filepath.Join(hw, "name"))))
			if preferred[name] {
				for n := 0; n < 8; n++ {
					p := filepath.Join(hw, fmt.Sprintf("temp%d_input", n))
					if s := strings.TrimSpace(readFile(p)); isDigitStr(s) {
						cpuTempPath = p
						return
					}
				}
			}
		}
		// Fallback: cerca per label
		for _, hw := range hwmons {
			for n := 0; n < 8; n++ {
				label := strings.ToLower(strings.TrimSpace(readFile(filepath.Join(hw, fmt.Sprintf("temp%d_label", n)))))
				if strings.Contains(label, "package") || strings.Contains(label, "tctl") || strings.Contains(label, "cpu") {
					p := filepath.Join(hw, fmt.Sprintf("temp%d_input", n))
					if isDigitStr(strings.TrimSpace(readFile(p))) {
						cpuTempPath = p
						return
					}
				}
			}
		}
	})
	return cpuTempPath
}

func cpuTempC() int {
	p := findCPUTempPath()
	if p == "" {
		return 0
	}
	s := strings.TrimSpace(readFile(p))
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	if v > 1000 {
		return v / 1000
	}
	return v
}

// ─── GPU ────────────────────────────────────────────────────────────────────

type GPUInfo struct {
	Name string
	Temp int
	Util int
}

var (
	gpuMu    sync.Mutex
	gpuCache GPUInfo
	gpuTS    time.Time
)

func getGPUInfo() GPUInfo {
	gpuMu.Lock()
	defer gpuMu.Unlock()
	if time.Since(gpuTS) < 2*time.Second {
		return gpuCache
	}
	gpuCache = fetchGPUInfo()
	gpuTS = time.Now()
	return gpuCache
}

func fetchGPUInfo() GPUInfo {
	// NVIDIA
	out := runCmd("nvidia-smi",
		"--query-gpu=name,temperature.gpu,utilization.gpu",
		"--format=csv,noheader,nounits")
	if out != "" {
		lines := strings.SplitN(strings.TrimSpace(out), "\n", 2)
		parts := strings.SplitN(lines[0], ",", 3)
		if len(parts) == 3 {
			temp, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			util, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
			return GPUInfo{Name: cleanGPUName(strings.TrimSpace(parts[0])), Temp: temp, Util: util}
		}
	}
	// AMD
	out = runCmd("rocm-smi", "--showtemp", "--showuse")
	if out != "" {
		re := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*c`)
		reU := regexp.MustCompile(`(\d+)\s*%`)
		temp, util := 0, 0
		if m := re.FindStringSubmatch(strings.ToLower(out)); m != nil {
			f, _ := strconv.ParseFloat(m[1], 64)
			temp = int(f)
		}
		if m := reU.FindStringSubmatch(out); m != nil {
			util, _ = strconv.Atoi(m[1])
		}
		return GPUInfo{Name: "AMD Radeon", Temp: temp, Util: util}
	}
	// Intel Arc / Iris Xe (driver i915 o xe)
	if info, ok := fetchIntelGPU(); ok {
		return info
	}
	// Fallback generico sysfs (qualunque card)
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]")
	for _, card := range cards {
		name := strings.TrimSpace(readFile(filepath.Join(card, "device", "product_name")))
		if name == "" {
			name = strings.TrimSpace(readFile(filepath.Join(card, "device", "name")))
		}
		if name == "" {
			continue
		}
		temp := 0
		if ms, _ := filepath.Glob(filepath.Join(card, "device", "hwmon", "hwmon*", "temp*_input")); len(ms) > 0 {
			if s := strings.TrimSpace(readFile(ms[0])); isDigitStr(s) {
				v, _ := strconv.Atoi(s)
				temp = v / 1000
			}
		}
		return GPUInfo{Name: cleanGPUName(name), Temp: temp}
	}
	return GPUInfo{Name: "GPU"}
}

// ─── Intel GPU (driver i915 / xe) ───────────────────────────────────────────

// intelCard restituisce il nome del drm card Intel (vendor 0x8086).
func intelCard() string {
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]")
	for _, card := range cards {
		if strings.TrimSpace(readFile(filepath.Join(card, "device", "vendor"))) == "0x8086" {
			return filepath.Base(card) // es. "card1"
		}
	}
	return ""
}

// intelGPUModelName legge il nome della GPU Intel da lspci.
func intelGPUModelName() string {
	out := runCmd("lspci")
	for _, line := range strings.Split(out, "\n") {
		l := strings.ToLower(line)
		if strings.Contains(l, "intel") && (strings.Contains(l, "vga") || strings.Contains(l, "display")) {
			// Formato: "00:02.0 VGA compatible controller: Intel Corporation Meteor Lake-P [Intel Arc Graphics] (rev 08)"
			if parts := strings.SplitN(line, ": ", 2); len(parts) == 2 {
				name := parts[1]
				name = regexp.MustCompile(`\s+\(rev [0-9a-f]+\)$`).ReplaceAllString(name, "")
				name = strings.ReplaceAll(name, "Intel Corporation ", "")
				// Estrai solo il nome tra parentesi quadre se presente: "Meteor Lake-P [Intel Arc Graphics]" → "Intel Arc Graphics"
				if m := regexp.MustCompile(`\[([^\]]+)\]`).FindStringSubmatch(name); m != nil {
					return strings.TrimSpace(m[1])
				}
				return strings.TrimSpace(name)
			}
		}
	}
	return "Intel GPU"
}

// intelGPUTempC cerca la temperatura nella hwmon del driver xe/i915.
// Su i915 Meteor Lake la temperatura GPU non è sempre esposta; ritorna 0 se assente.
func intelGPUTempC(card string) int {
	// Hwmon sotto il device della card (xe driver)
	base := fmt.Sprintf("/sys/class/drm/%s/device", card)
	if ms, _ := filepath.Glob(filepath.Join(base, "hwmon", "hwmon*", "temp*_input")); len(ms) > 0 {
		if s := strings.TrimSpace(readFile(ms[0])); isDigitStr(s) {
			v, _ := strconv.Atoi(s)
			if v > 1000 {
				return v / 1000
			}
			return v
		}
	}
	// Hwmon globale named "xe" o "i915"
	hws, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, hw := range hws {
		n := strings.ToLower(strings.TrimSpace(readFile(filepath.Join(hw, "name"))))
		if n == "xe" || n == "i915" {
			for i := 1; i < 8; i++ {
				p := filepath.Join(hw, fmt.Sprintf("temp%d_input", i))
				if s := strings.TrimSpace(readFile(p)); isDigitStr(s) {
					v, _ := strconv.Atoi(s)
					if v > 1000 {
						return v / 1000
					}
					return v
				}
			}
		}
	}
	return 0
}

// intelGPUFreqMHz legge frequenza corrente e massima in MHz (i915 e xe).
func intelGPUFreqMHz(card string) (cur, max int) {
	// i915: /sys/class/drm/<card>/device/drm/<card>/gt/gt0/
	i915Base := fmt.Sprintf("/sys/class/drm/%s/device/drm/%s/gt/gt0", card, card)
	if s := strings.TrimSpace(readFile(filepath.Join(i915Base, "rps_cur_freq_mhz"))); isDigitStr(s) {
		cur, _ = strconv.Atoi(s)
		if s2 := strings.TrimSpace(readFile(filepath.Join(i915Base, "rps_max_freq_mhz"))); isDigitStr(s2) {
			max, _ = strconv.Atoi(s2)
		}
		return
	}
	// xe: /sys/class/drm/<card>/device/tile0/gt0/freq0/
	xeBase := fmt.Sprintf("/sys/class/drm/%s/device/tile0/gt0/freq0", card)
	if s := strings.TrimSpace(readFile(filepath.Join(xeBase, "cur_freq"))); isDigitStr(s) {
		cur, _ = strconv.Atoi(s)
		if s2 := strings.TrimSpace(readFile(filepath.Join(xeBase, "max_freq"))); isDigitStr(s2) {
			max, _ = strconv.Atoi(s2)
		}
	}
	return
}

// intelGPUUtil prova intel_gpu_top (JSON) se installato;
// altrimenti stima l'utilizzo dalla frequenza corrente/massima.
func intelGPUUtil(card string) int {
	// Prova intel_gpu_top -J (campione 300ms)
	ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "intel_gpu_top", "-J", "-s", "300").Output()
	if err == nil && len(out) > 10 {
		if u := parseIntelGpuTopJSON(out); u >= 0 {
			return u
		}
	}
	// Fallback: stima da frequenza (cur/max * 100)
	cur, max := intelGPUFreqMHz(card)
	if max > 0 {
		return cur * 100 / max
	}
	return 0
}

// parseIntelGpuTopJSON estrae il % busy massimo tra tutti gli engine dal JSON di intel_gpu_top.
func parseIntelGpuTopJSON(data []byte) int {
	depth, start := 0, -1
	for i, b := range data {
		switch b {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				var result struct {
					Engines map[string]struct {
						Busy float64 `json:"busy"`
					} `json:"engines"`
				}
				if json.Unmarshal(data[start:i+1], &result) == nil {
					maxBusy := 0.0
					for _, e := range result.Engines {
						if e.Busy > maxBusy {
							maxBusy = e.Busy
						}
					}
					return int(math.Round(maxBusy))
				}
				start = -1
			}
		}
	}
	return -1
}

// fetchIntelGPU rileva e legge le metriche della GPU Intel.
func fetchIntelGPU() (GPUInfo, bool) {
	card := intelCard()
	if card == "" {
		return GPUInfo{}, false
	}
	return GPUInfo{
		Name: intelGPUModelName(),
		Temp: intelGPUTempC(card),
		Util: intelGPUUtil(card),
	}, true
}

func cleanGPUName(name string) string {
	re := regexp.MustCompile(`(?i)\(R\)|\(TM\)|NVIDIA Corporation|Advanced Micro Devices,? Inc\.?|Intel\(R\)\s*`)
	s := strings.TrimSpace(re.ReplaceAllString(name, ""))
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	if s == "" {
		return "GPU"
	}
	return s
}

// ─── Memoria ────────────────────────────────────────────────────────────────

type MemInfo struct {
	Used, Avail, Total float64
	UsagePct           int
}

func getMemInfo() MemInfo {
	content := readFile("/proc/meminfo")
	vals := map[string]int64{}
	for _, line := range strings.Split(content, "\n") {
		parts := strings.Fields(strings.ReplaceAll(line, ":", ""))
		if len(parts) >= 2 {
			if v, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				vals[parts[0]] = v
			}
		}
	}
	total := vals["MemTotal"]
	avail := vals["MemAvailable"]
	used := total - avail
	if used < 0 {
		used = 0
	}
	toGB := func(kb int64) float64 {
		return math.Round(float64(kb)/1024/1024*10) / 10
	}
	pct := 0
	if total > 0 {
		pct = int(math.Round(100 * float64(used) / float64(total)))
	}
	return MemInfo{Used: toGB(used), Avail: toGB(avail), Total: toGB(total), UsagePct: pct}
}

var (
	ramLabelOnce  sync.Once
	ramLabelCache string
)

func getRamLabel() string {
	ramLabelOnce.Do(func() {
		out := runCmd("dmidecode", "-t", "memory")
		if out == "" {
			out = runCmd("sudo", "-n", "dmidecode", "-t", "memory")
		}
		if out != "" {
			re := regexp.MustCompile(`(?im)^\s*Manufacturer:\s*(.+)$`)
			if m := re.FindStringSubmatch(out); m != nil {
				manu := strings.TrimSpace(m[1])
				bad := map[string]bool{"Undefined": true, "Not Specified": true, "Unknown": true, "To Be Filled By O.E.M.": true}
				if !bad[manu] {
					manu = strings.ReplaceAll(manu, "Micron Technology", "Micron")
					manu = strings.ReplaceAll(manu, "Samsung Electronics", "Samsung")
					manu = strings.ReplaceAll(strings.ReplaceAll(manu, "HYNIX", "SK hynix"), "Hynix", "SK hynix")
					ramLabelCache = manu
					return
				}
			}
		}
		ramLabelCache = "Memory"
	})
	return ramLabelCache
}

// ─── Disco ──────────────────────────────────────────────────────────────────

type DiskInfo struct {
	Used, Total int
	UsagePct    int
}

func getDiskInfo() DiskInfo {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return DiskInfo{}
	}
	frsize := uint64(st.Frsize)
	totalB := st.Blocks * frsize
	availB := st.Bavail * frsize
	usedB := totalB - availB
	toGB := func(b uint64) int { return int(math.Round(float64(b) / 1e9)) }
	pct := 0
	if totalB > 0 {
		pct = int(math.Round(100 * float64(usedB) / float64(totalB)))
	}
	return DiskInfo{Used: toGB(usedB), Total: toGB(totalB), UsagePct: pct}
}

var (
	diskLabelOnce  sync.Once
	diskLabelCache string
)

func getDiskLabel() string {
	diskLabelOnce.Do(func() {
		// NVMe via sysfs
		if matches, _ := filepath.Glob("/sys/class/nvme/nvme*"); len(matches) > 0 {
			if model := strings.TrimSpace(readFile(filepath.Join(matches[0], "model"))); model != "" {
				diskLabelCache = model
				return
			}
		}
		diskLabelCache = "Disk"
	})
	return diskLabelCache
}

var (
	diskTempOnce sync.Once
	diskTempPath string
)

func getDiskTempC() int {
	diskTempOnce.Do(func() {
		hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
		for _, hw := range hwmons {
			if strings.TrimSpace(readFile(filepath.Join(hw, "name"))) == "nvme" {
				for n := 0; n < 4; n++ {
					p := filepath.Join(hw, fmt.Sprintf("temp%d_input", n))
					if isDigitStr(strings.TrimSpace(readFile(p))) {
						diskTempPath = p
						return
					}
				}
			}
		}
	})
	if diskTempPath == "" {
		return 0
	}
	s := strings.TrimSpace(readFile(diskTempPath))
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	if v > 1000 {
		return v / 1000
	}
	return v
}

// ─── Rete ───────────────────────────────────────────────────────────────────

type netMeter struct {
	mu    sync.Mutex
	iface string
	rx0   uint64
	tx0   uint64
	t0    time.Time
}

var globalNet = &netMeter{}

func init() {
	globalNet.prime("")
}

func (m *netMeter) prime(preferred string) {
	iface := pickIface(preferred)
	m.iface = iface
	if iface == "" {
		return
	}
	rx, tx := parseNetDev(iface)
	m.rx0, m.tx0, m.t0 = rx, tx, time.Now()
}

func (m *netMeter) Rates(preferred string) (rxKBs, txKBs float64, iface string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if preferred != "" && preferred != m.iface {
		m.prime(preferred)
	}
	if m.iface == "" {
		m.prime(preferred)
	}

	rx1, tx1 := parseNetDev(m.iface)
	now := time.Now()
	dt := now.Sub(m.t0).Seconds()
	if dt < 0.001 {
		dt = 0.001
	}

	rxKBs = float64(rx1-m.rx0) / dt / 1024
	txKBs = float64(tx1-m.tx0) / dt / 1024
	if rxKBs < 0 {
		rxKBs = 0
	}
	if txKBs < 0 {
		txKBs = 0
	}

	m.rx0, m.tx0, m.t0 = rx1, tx1, now
	return rxKBs, txKBs, m.iface
}

func pickIface(preferred string) string {
	if preferred != "" {
		return preferred
	}
	out := runCmd("ip", "-o", "route", "show", "default")
	re := regexp.MustCompile(`\bdev\s+(\S+)`)
	for _, line := range strings.Split(out, "\n") {
		if m := re.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	entries, _ := os.ReadDir("/sys/class/net")
	for _, e := range entries {
		if e.Name() != "lo" {
			return e.Name()
		}
	}
	return ""
}

func parseNetDev(iface string) (uint64, uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if strings.TrimSpace(parts[0]) != iface {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(parts[1]))
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		return rx, tx
	}
	return 0, 0
}

func fmtRate(kbs float64) string {
	if kbs < 1024 {
		return fmt.Sprintf("%.1f K/s", kbs)
	}
	mbs := kbs / 1024
	if mbs < 1024 {
		return fmt.Sprintf("%.1f M/s", mbs)
	}
	return fmt.Sprintf("%.1f G/s", mbs/1024)
}

// ─── Ventola ────────────────────────────────────────────────────────────────

var (
	fanHwmonOnce sync.Once
	fanHwmonPath string
	fanHistory   []int
)

func findFanHwmonPath() string {
	fanHwmonOnce.Do(func() {
		matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/fan*_input")
		for _, p := range matches {
			if s := strings.TrimSpace(readFile(p)); isDigitStr(s) {
				if v, _ := strconv.Atoi(s); v > 0 {
					fanHwmonPath = p
					return
				}
			}
		}
	})
	return fanHwmonPath
}

func getFanRPM(prefer string, maxRPM int) int {
	var rpm int = -1

	readHwmon := func() int {
		p := findFanHwmonPath()
		if p == "" {
			return -1
		}
		if s := strings.TrimSpace(readFile(p)); isDigitStr(s) {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				return v
			}
		}
		return -1
	}

	readNvidia := func() int {
		out := runCmd("nvidia-smi",
			"--query-gpu=fan.speed", "--format=csv,noheader,nounits")
		if out == "" {
			return -1
		}
		pct, err := strconv.Atoi(strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]))
		if err != nil {
			return -1
		}
		return pct * maxRPM / 100
	}

	switch strings.ToLower(prefer) {
	case "hwmon":
		if rpm = readHwmon(); rpm < 0 {
			rpm = readNvidia()
		}
	case "nvidia":
		if rpm = readNvidia(); rpm < 0 {
			rpm = readHwmon()
		}
	default: // auto: hwmon prima, poi nvidia
		if rpm = readHwmon(); rpm < 0 {
			rpm = readNvidia()
		}
	}

	if rpm > 0 {
		fanHistory = append(fanHistory, rpm)
		if len(fanHistory) > 5 {
			fanHistory = fanHistory[1:]
		}
		sum := 0
		for _, v := range fanHistory {
			sum += v
		}
		return sum / len(fanHistory)
	}
	return -1
}

// ─── Volume ─────────────────────────────────────────────────────────────────

func getVolume() int {
	out := runCmd("pactl", "get-sink-volume", "@DEFAULT_SINK@")
	re := regexp.MustCompile(`(\d+)%`)
	if m := re.FindStringSubmatch(out); m != nil {
		v, _ := strconv.Atoi(m[1])
		return v
	}
	return -1
}

// ─── Batteria ───────────────────────────────────────────────────────────────

func getBattery() int {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return 177 // sentinella: nessuna batteria (desktop)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "BAT") {
			s := strings.TrimSpace(readFile("/sys/class/power_supply/" + e.Name() + "/capacity"))
			if v, err := strconv.Atoi(s); err == nil {
				return v
			}
		}
	}
	return 177
}
