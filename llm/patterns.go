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
	sb.Grow(1024 + len(patterns)*150) // Use larger buffer

	sb.WriteString("Anda adalah analis pasar saham expert spesialis Bandarlogy (Analisis Arus Dana Institusi). Analisis pola berikut:\n\n")

	for i, p := range patterns {
		duration := p.LastAlertTime.Sub(p.FirstAlertTime).Minutes()
		sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, p.StockSymbol, p.Action))
		sb.WriteString(fmt.Sprintf("   - Intensitas: %d kali 'HAKA' dalam %.0f menit\n", p.AlertCount, duration))
		sb.WriteString(fmt.Sprintf("   - Total Value: Rp %.2f Miliar\n", p.TotalValue/billionDivisor))
		sb.WriteString(fmt.Sprintf("   - Rata-rata Kekuatan (Z-Score): %.2f\n\n", p.AvgZScore))
	}

	sb.WriteString("Tugas Analisis:\n")
	sb.WriteString("1. **Identifikasi Fase**: Apakah ini fase Akumulasi (markup awal) atau Distribusi (markdown)?\n")
	sb.WriteString("2. **Validasi Kekuatan**: Apakah Z-Score mendukung kenaikan harga yang berkelanjutan atau hanya 'fake pump'?\n")
	sb.WriteString("3. **Rencana Trading**: Entry point ideal dan stop loss rasional berdasarkan volatilitas ini.\n")
	sb.WriteString(fmt.Sprintf("\nJawab dalam Bahasa Indonesia yang tajam, to-the-point, dan profesional. Maksimal %d kata.", maxPromptWords))

	return sb.String()
}

// FormatAnomalyPrompt creates a prompt for analyzing extreme Z-score events
func FormatAnomalyPrompt(anomalies []database.WhaleAlert) string {
	var sb strings.Builder
	sb.Grow(1024 + len(anomalies)*200)

	sb.WriteString("Anda mendeteksi anomali statistik ekstrem (Black Swan Event) di pasar saham. Analisis data berikut:\n\n")

	for i, a := range anomalies {
		if i >= maxAnomalies {
			break
		}

		zScore := safeFloat64(a.ZScore, 0.0)
		volPct := safeFloat64(a.VolumeVsAvgPct, 0.0)
		timeSince := time.Since(a.DetectedAt).Minutes()

		sb.WriteString(fmt.Sprintf("%d. **%s** - %s Ekstrem!\n", i+1, a.StockSymbol, a.Action))
		sb.WriteString(fmt.Sprintf("   - Waktu: %.0f menit yang lalu\n", timeSince))
		sb.WriteString(fmt.Sprintf("   - Deviasi Standar (Z-Score): %.2f (Sangat Langka)\n", zScore))
		sb.WriteString(fmt.Sprintf("   - Ledakan Volume: %.0f%% diatas rata-rata\n", volPct))
		sb.WriteString(fmt.Sprintf("   - Nilai Transaksi: Rp %.2f Juta di harga %.0f\n\n", a.TriggerValue/millionDivisor, a.TriggerPrice))
	}

	sb.WriteString("Berikan Analisis Forensik:\n")
	sb.WriteString("1. **Penyebab Anomali**: Apakah ini 'Crossing' besar, 'Panic Selling', atau 'Aggressive Buying'?\n")
	sb.WriteString("2. **Psikologi Pasar**: Apa yang sedang dipikirkan pelaku pasar (FOMO vs FEAR)?\n")
	sb.WriteString("3. **Keputusan Cepat**: Ikuti arus (Follow the Whale) atau hindari volatilitas (Wait and See)?\n")
	sb.WriteString("\nBerikan insight seolah-olah Anda adalah hedge fund manager. Singkat & padat.")

	return sb.String()
}

// FormatTimingPrompt creates a prompt for time-based pattern analysis
func FormatTimingPrompt(stats []database.TimeBasedStat) string {
	var sb strings.Builder
	sb.Grow(1024 + len(stats)*100)

	sb.WriteString("Analisis kebiasaan waktu transaksi (Time-Series Behavior) dari Big Player:\n\n")

	for _, s := range stats {
		hour := s.TimeBucket
		sb.WriteString(fmt.Sprintf("🕒 **Jam %s:00**\n", hour))
		sb.WriteString(fmt.Sprintf("   - Aktivitas: %d alert (Beli: %d | Jual: %d)\n", s.AlertCount, s.BuyCount, s.SellCount))
		sb.WriteString(fmt.Sprintf("   - Total Perputaran Uang: Rp %.1f Miliar\n", s.TotalValue/billionDivisor))
	}

	sb.WriteString("\nEvaluasi Strategis:\n")
	sb.WriteString("1. **Jam Krusial**: Kapan waktu paling berbahaya (volatilitas tinggi) dan kapan waktu teraman?\n")
	sb.WriteString("2. **Pola Intraday**: Apakah ada pola 'Morning Push' atau 'Closing Dump'?\n")
	sb.WriteString("3. **Taktik Timing**: Kapan trader retail sebaiknya masuk untuk menghindari 'gap' harga?\n")
	sb.WriteString("\nJawab dengan gaya mentoring trading profesional.")

	return sb.String()
}

// AnalyzeSymbolContext generates LLM insights for a specific stock
func AnalyzeSymbolContext(client *Client, symbol string, alerts []database.WhaleAlert) (string, error) {
	if len(alerts) == 0 {
		return "", fmt.Errorf("tidak ada data aktivitas whale yang cukup untuk analisis %s", symbol)
	}

	var sb strings.Builder
	sb.Grow(1024)
	sb.WriteString(fmt.Sprintf("Lakukan Bedah Saham (Stock Opname) untuk **%s** berdasarkan aliran dana Bandar:\n\n", symbol))

	counts := countAlerts(alerts, false)

	sb.WriteString(fmt.Sprintf("- 🟢 **Tekanan Beli (Accumulation)**: %d transaksi (Total Rp %.2f M)\n", counts.buyCount, counts.totalBuyValue/billionDivisor))
	sb.WriteString(fmt.Sprintf("- 🔴 **Tekanan Jual (Distribution)**: %d transaksi (Total Rp %.2f M)\n", counts.sellCount, counts.totalSellValue/billionDivisor))
	if counts.unknownCount > 0 {
		sb.WriteString(fmt.Sprintf("- ⚪ **Netral/Crossing**: %d transaksi (Total Rp %.2f M)\n", counts.unknownCount, counts.totalUnknownValue/billionDivisor))
	}

	// Add trend context if available
	if counts.totalBuyValue > counts.totalSellValue*1.5 {
		sb.WriteString("\nKonteks: Dominasi BUY sangat kuat (>1.5x Sell).\n")
	} else if counts.totalSellValue > counts.totalBuyValue*1.5 {
		sb.WriteString("\nKonteks: Dominasi SELL sangat kuat (>1.5x Buy).\n")
	} else {
		sb.WriteString("\nKonteks: Pertarungan seimbang (Konsolidasi).\n")
	}

	sb.WriteString("\nKesimpulan Cepat (Micro-Analysis):\n")
	sb.WriteString("1. Siapa yang memegang kendali (Buyer vs Seller)?\n")
	sb.WriteString("2. Prediksi pergerakan jangka pendek (Bounce/Breakdown)?\n")
	sb.WriteString("3. Skor Potensi (1-10)?\n")
	sb.WriteString("Jawab <100 kata.")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second) // Increased timeout slightly
	defer cancel()

	return client.Analyze(ctx, sb.String())
}

// FormatSymbolAnalysisPrompt creates a detailed prompt for symbol-specific streaming analysis
func FormatSymbolAnalysisPrompt(symbol string, alerts []database.WhaleAlert) string {
	var sb strings.Builder
	sb.Grow(2048 + len(alerts)*50)

	sb.WriteString(fmt.Sprintf("Anda adalah AI Quantum Trader. Lakukan Deep Dive Analysis pada saham **%s**:\n\n", symbol))

	if len(alerts) == 0 {
		sb.WriteString("Tidak ada jejak aktivitas Big Player terdeteksi saat ini. Market sepi/Retail dominan.\n")
		return sb.String()
	}

	counts := countAlerts(alerts, true)
	totalVal := counts.totalBuyValue + counts.totalSellValue + counts.totalUnknownValue

	// Summary Statistic
	sb.WriteString(fmt.Sprintf("📊 **Peta Kekuatan Bandar (%d Transaksi Terakhir)**:\n", len(alerts)))
	sb.WriteString(fmt.Sprintf("Volume Total: Rp %.1f Miliar\n", totalVal/billionDivisor))
	sb.WriteString(fmt.Sprintf("- 🐂 Bulls (Buy): %d ord | Rp %.1f M\n", counts.buyCount, counts.totalBuyValue/millionDivisor))
	sb.WriteString(fmt.Sprintf("- 🐻 Bears (Sell): %d ord | Rp %.1f M\n", counts.sellCount, counts.totalSellValue/millionDivisor))
	sb.WriteString("\n")

	// Whale Tracking
	sb.WriteString("🐋 **Whale Tracking (Transaksi Raksasa)**:\n")
	if counts.maxBuyValue > 0 {
		sb.WriteString(fmt.Sprintf("✅ **Top Buy**: Rp %.1f M", counts.maxBuyValue/millionDivisor))
		if counts.maxBuyAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Power Z: %.2f)", *counts.maxBuyAlert.ZScore))
		}
		sb.WriteString("\n")
	}
	if counts.maxSellValue > 0 {
		sb.WriteString(fmt.Sprintf("❌ **Top Sell**: Rp %.1f M", counts.maxSellValue/millionDivisor))
		if counts.maxSellAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Power Z: %.2f)", *counts.maxSellAlert.ZScore))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n**Executive Summary & Strategy**:\n")
	sb.WriteString("1. **Diagnosa Trend**: Apakah Bandar sedang Akumulasi diam-diam, Markup harga, atau Distribusi muatan?\n")
	sb.WriteString("2. **Key Levels**: Di harga berapa 'smart money' banyak masuk? (Jadikan ini support kuat).\n")
	sb.WriteString("3. **Trading Plan**: \n")
	sb.WriteString("   - **Action**: BUY / WAIT / SELL / AVOID\n")
	sb.WriteString("   - **Reason**: Alasan logis berdasarkan data arus uang di atas.\n")
	sb.WriteString(fmt.Sprintf("\nBerikan analisis mendalam namun mudah dicerna untuk trader profesional. Maksimal %d kata.", maxPromptWords))

	return sb.String()
}
