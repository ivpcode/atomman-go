package main

import (
	"fmt"
	"strings"
	"time"
)

type tileDesc struct {
	ID        byte
	Name      string
	PayloadFn func(cfg *Config) string
}

func buildTileRotation(cfg *Config) []tileDesc {
	all := map[string]tileDesc{
		"cpu":      {ID: TileCPU, Name: "cpu", PayloadFn: payloadCPU},
		"gpu":      {ID: TileGPU, Name: "gpu", PayloadFn: payloadGPU},
		"memory":   {ID: TileMEM, Name: "memory", PayloadFn: payloadMem},
		"disk":     {ID: TileDSK, Name: "disk", PayloadFn: payloadDisk},
		"datetime": {ID: TileDATE, Name: "datetime", PayloadFn: payloadDate},
		"network":  {ID: TileNET, Name: "network", PayloadFn: payloadNet},
		"volume":   {ID: TileVOL, Name: "volume", PayloadFn: payloadVolume},
		"battery":  {ID: TileBAT, Name: "battery", PayloadFn: payloadBattery},
	}

	var rotation []tileDesc
	for _, name := range cfg.Tiles {
		if td, ok := all[strings.ToLower(strings.TrimSpace(name))]; ok {
			rotation = append(rotation, td)
		} else {
			fmt.Printf("[WARN] Tile sconosciuto nel config: %q\n", name)
		}
	}
	return rotation
}

func payloadCPU(cfg *Config) string {
	name := cfg.CustomLabels.CPU
	if name == "" {
		name = cpuModel()
	}
	temp := cpuTempC()
	return fmt.Sprintf("{CPU:%s;Tempr:%d;Useage:%d;Freq:%d;Tempr1:%d;}",
		name, temp, cpuUsagePct(), cpuFreqKHz(), temp)
}

func payloadGPU(cfg *Config) string {
	name := cfg.CustomLabels.GPU
	g := getGPUInfo()
	if name == "" {
		name = g.Name
	}
	if g.Temp == 0 {
		return fmt.Sprintf("{GPU:%s;Tempr:;Useage:%d}", name, g.Util)
	}
	return fmt.Sprintf("{GPU:%s;Tempr:%d;Useage:%d}", name, g.Temp, g.Util)
}

func payloadMem(cfg *Config) string {
	name := cfg.CustomLabels.Memory
	if name == "" {
		name = getRamLabel()
	}
	m := getMemInfo()
	return fmt.Sprintf("{Memory:%s;Used:%.1f;Available:%.1f;Total:%.1f;Useage:%d}",
		name, m.Used, m.Avail, m.Total, m.UsagePct)
}

func payloadDisk(cfg *Config) string {
	name := cfg.CustomLabels.Disk
	if name == "" {
		name = getDiskLabel()
	}
	d := getDiskInfo()
	temp := getDiskTempC()
	return fmt.Sprintf("{DiskName:%s;Tempr:%d;UsageSpace:%d;AllSpace:%d;Usage:%d}",
		name, temp, d.Used, d.Total, d.UsagePct)
}

func payloadDate(cfg *Config) string {
	t := time.Now()
	week := int(t.Weekday())
	w := GetWeather(cfg)
	if w != nil {
		return fmt.Sprintf(
			"{Date:%04d/%02d/%02d;Time:%02d:%02d:%02d;Week:%d;Weather:%d;TemprLo:%d,TemprHi:%d,Zone:%s,Desc:%s}",
			t.Year(), t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second(),
			week, w.Code, w.Lo, w.Hi, w.Zone, w.Desc,
		)
	}
	return fmt.Sprintf(
		"{Date:%04d/%02d/%02d;Time:%02d:%02d:%02d;Week:%d;Weather:;TemprLo:,TemprHi:,Zone:,Desc:}",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(),
		week,
	)
}

func payloadNet(cfg *Config) string {
	rxKBs, txKBs, _ := globalNet.Rates(cfg.Network.Interface)
	rpm := getFanRPM(cfg.Fan.Prefer, cfg.Fan.MaxRPM)

	netStr := "N/A,N/A"
	if rxKBs > 0 || txKBs > 0 {
		netStr = fmt.Sprintf("%s,%s", fmtRate(rxKBs), fmtRate(txKBs))
	}
	return fmt.Sprintf("{SPEED:%d;NETWORK:%s}", rpm, netStr)
}

func payloadVolume(_ *Config) string {
	return fmt.Sprintf("{VOLUME:%d}", getVolume())
}

func payloadBattery(_ *Config) string {
	return fmt.Sprintf("{Battery:%d}", getBattery())
}
