package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"stockbit-haka-haki/database"
)

// Constants for value formatting
const (
	billionDivisor = 1_000_000_000
	millionDivisor = 1_000_000
	maxAnomalies   = 10
	maxPromptWords = 200
)

// alertCounts aggregates alert statistics by action type
type alertCounts struct {
	buyCount          int
	sellCount         int
	unknownCount      int
	totalBuyValue     float64
	totalSellValue    float64
	totalUnknownValue float64
	maxBuyAlert       database.WhaleAlert
	maxSellAlert      database.WhaleAlert
	maxUnknownAlert   database.WhaleAlert
	maxBuyValue       float64
	maxSellValue      float64
	maxUnknownValue   float64
}

// countAlerts processes a list of whale alerts and returns aggregated statistics
func countAlerts(alerts []database.WhaleAlert, trackMax bool) alertCounts {
	counts := alertCounts{}

	for _, a := range alerts {
		switch a.Action {
		case "BUY":
			counts.buyCount++
			counts.totalBuyValue += a.TriggerValue
			if trackMax && a.TriggerValue > counts.maxBuyValue {
				counts.maxBuyValue = a.TriggerValue
				counts.maxBuyAlert = a
			}
		case "SELL":
			counts.sellCount++
			counts.totalSellValue += a.TriggerValue
			if trackMax && a.TriggerValue > counts.maxSellValue {
				counts.maxSellValue = a.TriggerValue
				counts.maxSellAlert = a
			}
		default:
			counts.unknownCount++
			counts.totalUnknownValue += a.TriggerValue
			if trackMax && a.TriggerValue > counts.maxUnknownValue {
				counts.maxUnknownValue = a.TriggerValue
				counts.maxUnknownAlert = a
			}
		}
	}

	return counts
}

// safeFloat64 safely dereferences a float64 pointer, returning defaultValue if nil
func safeFloat64(ptr *float64, defaultValue float64) float64 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// FormatAccumulationPrompt creates a prompt for LLM to analyze accumulation/distribution patterns
func FormatAccumulationPrompt(patterns []database.AccumulationPattern) string {
	var sb strings.Builder
	sb.Grow(512 + len(patterns)*150) // Pre-allocate capacity

	sb.WriteString("Analisis pola trading whale berikut yang terdeteksi di pasar saham Indonesia:\n\n")

	for i, p := range patterns {
		duration := p.LastAlertTime.Sub(p.FirstAlertTime).Minutes()
		sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, p.StockSymbol, p.Action))
		sb.WriteString(fmt.Sprintf("   - Alert: %d kali dalam %.0f menit\n", p.AlertCount, duration))
		sb.WriteString(fmt.Sprintf("   - Total Nilai: Rp %.2f\n", p.TotalValue/billionDivisor))
		sb.WriteString(fmt.Sprintf("   - Rata-rata Z-Score: %.2f\n\n", p.AvgZScore))
	}

	sb.WriteString("Berikan:\n")
	sb.WriteString("1. Insight utama - apakah ini pola akumulasi atau distribusi?\n")
	sb.WriteString("2. Rekomendasi actionable untuk trader retail\n")
	sb.WriteString("3. Penilaian risiko\n")
	sb.WriteString(fmt.Sprintf("\nJawaban maksimal %d kata. Ringkas dan actionable.", maxPromptWords))

	return sb.String()
}

// FormatAnomalyPrompt creates a prompt for analyzing extreme Z-score events
func FormatAnomalyPrompt(anomalies []database.WhaleAlert) string {
	var sb strings.Builder
	sb.Grow(512 + len(anomalies)*200) // Pre-allocate capacity

	sb.WriteString("Analisis anomali trading whale ekstrem berikut (Z-Score > 5.0 = probabilitas 0.00003%):\n\n")

	for i, a := range anomalies {
		if i >= maxAnomalies {
			break
		}

		zScore := safeFloat64(a.ZScore, 0.0)
		volPct := safeFloat64(a.VolumeVsAvgPct, 0.0)
		timeSince := time.Since(a.DetectedAt).Minutes()

		sb.WriteString(fmt.Sprintf("%d. %s %s - %.0f menit yang lalu\n", i+1, a.StockSymbol, a.Action, timeSince))
		sb.WriteString(fmt.Sprintf("   - Z-Score: %.2f (%.0f%% vs volume rata-rata)\n", zScore, volPct))
		sb.WriteString(fmt.Sprintf("   - Nilai: Rp %.2f Juta di harga %.0f\n\n", a.TriggerValue/millionDivisor, a.TriggerPrice))
	}

	sb.WriteString("Apa yang diindikasikan oleh anomali ekstrem ini? Apakah:\n")
	sb.WriteString("1. Posisi institusional?\n")
	sb.WriteString("2. Potensi manipulasi?\n")
	sb.WriteString("3. Reaksi berita?\n")
	sb.WriteString("\nBerikan analisis singkat dan actionable maksimal 150 kata.")

	return sb.String()
}

// FormatTimingPrompt creates a prompt for time-based pattern analysis
func FormatTimingPrompt(stats []database.TimeBasedStat) string {
	var sb strings.Builder
	sb.Grow(512 + len(stats)*100) // Pre-allocate capacity

	sb.WriteString("Distribusi aktivitas whale berdasarkan jam dalam sehari:\n\n")

	for _, s := range stats {
		hour := s.TimeBucket
		sb.WriteString(fmt.Sprintf("Jam %s:00 - %d alert (Beli: %d, Jual: %d), Nilai: Rp %.1fM\n",
			hour, s.AlertCount, s.BuyCount, s.SellCount, s.TotalValue/billionDivisor))
	}

	sb.WriteString("\nAnalisis:\n")
	sb.WriteString("1. Kapan whale paling aktif?\n")
	sb.WriteString("2. Interpretasi pola (posisi pembukaan, sepi saat makan siang, aksi closing?)\n")
	sb.WriteString("3. Waktu terbaik untuk trader retail memantau\n")
	sb.WriteString("\nMaksimal 150 kata.")

	return sb.String()
}

// AnalyzeSymbolContext generates LLM insights for a specific stock
func AnalyzeSymbolContext(client *Client, symbol string, alerts []database.WhaleAlert) (string, error) {
	if len(alerts) == 0 {
		return "", fmt.Errorf("tidak ada alert untuk dianalisis")
	}

	var sb strings.Builder
	sb.Grow(512) // Pre-allocate capacity
	sb.WriteString(fmt.Sprintf("Aktivitas whale terbaru untuk %s:\n\n", symbol))

	counts := countAlerts(alerts, false)

	sb.WriteString(fmt.Sprintf("- %d alert BUY (Rp %.2fM)\n", counts.buyCount, counts.totalBuyValue/billionDivisor))
	sb.WriteString(fmt.Sprintf("- %d alert SELL (Rp %.2fM)\n", counts.sellCount, counts.totalSellValue/billionDivisor))
	if counts.unknownCount > 0 {
		sb.WriteString(fmt.Sprintf("- %d alert NEGO/UNKNOWN (Rp %.2fM)\n", counts.unknownCount, counts.totalUnknownValue/billionDivisor))
	}
	sb.WriteString("\nApa sentimen marketnya? Bullish/Bearish? Berikan analisis singkat (<100 kata).")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.Analyze(ctx, sb.String())
}

// FormatSymbolAnalysisPrompt creates a detailed prompt for symbol-specific streaming analysis
func FormatSymbolAnalysisPrompt(symbol string, alerts []database.WhaleAlert) string {
	var sb strings.Builder
	sb.Grow(1024 + len(alerts)*50) // Pre-allocate capacity

	sb.WriteString(fmt.Sprintf("Analisis mendalam aktivitas whale untuk saham **%s**:\n\n", symbol))

	if len(alerts) == 0 {
		sb.WriteString("Tidak ada aktivitas whale terdeteksi dalam periode ini.\n")
		return sb.String()
	}

	counts := countAlerts(alerts, true)

	// Summary
	sb.WriteString(fmt.Sprintf("📊 **Ringkasan (%d alert terakhir)**:\n", len(alerts)))
	sb.WriteString(fmt.Sprintf("- BUY: %d transaksi (Total: Rp %.1fM)\n", counts.buyCount, counts.totalBuyValue/millionDivisor))
	sb.WriteString(fmt.Sprintf("- SELL: %d transaksi (Total: Rp %.1fM)\n", counts.sellCount, counts.totalSellValue/millionDivisor))
	if counts.unknownCount > 0 {
		sb.WriteString(fmt.Sprintf("- NEGO/UNKNOWN: %d transaksi (Total: Rp %.1fM)\n", counts.unknownCount, counts.totalUnknownValue/millionDivisor))
	}
	sb.WriteString("\n")

	// Largest transactions
	if counts.maxBuyValue > 0 {
		sb.WriteString(fmt.Sprintf("💰 **Akumulasi Terbesar**: Rp %.1fM", counts.maxBuyValue/millionDivisor))
		if counts.maxBuyAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Z-Score: %.2f)", *counts.maxBuyAlert.ZScore))
		}
		sb.WriteString("\n")
	}
	if counts.maxSellValue > 0 {
		sb.WriteString(fmt.Sprintf("📉 **Distribusi Terbesar**: Rp %.1fM", counts.maxSellValue/millionDivisor))
		if counts.maxSellAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Z-Score: %.2f)", *counts.maxSellAlert.ZScore))
		}
		sb.WriteString("\n")
	}
	if counts.maxUnknownValue > 0 {
		sb.WriteString(fmt.Sprintf("❓ **Transaksi NEGO/UNKNOWN Terbesar**: Rp %.1fM", counts.maxUnknownValue/millionDivisor))
		if counts.maxUnknownAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Z-Score: %.2f)", *counts.maxUnknownAlert.ZScore))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n**Analisis**:\n")
	sb.WriteString("1. Apa sentimen dominan (Bullish/Bearish/Netral)?\n")
	sb.WriteString("2. Apakah whale sedang akumulasi atau distribusi?\n")
	sb.WriteString("3. Level support/resistance potensial berdasarkan aktivitas?\n")
	sb.WriteString("4. Rekomendasi untuk trader retail\n")
	sb.WriteString(fmt.Sprintf("\nMaksimal %d kata, gunakan bahasa yang mudah dipahami.", maxPromptWords))

	return sb.String()
}
