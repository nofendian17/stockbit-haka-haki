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
	sb.Grow(1024 + len(patterns)*200)

	sb.WriteString("Anda adalah analis pasar saham expert spesialis Bandarlogy (Analisis Arus Dana Institusi). Analisis pola berikut berdasarkan DATA FAKTUAL:\n\n")

	for i, p := range patterns {
		duration := p.LastAlertTime.Sub(p.FirstAlertTime).Minutes()

		// Calculate derived metrics
		// TotalValue is in Rp, TotalVolumeLots is in lots (1 lot = 100 shares)
		// AvgPrice = TotalValue / (TotalVolumeLots * 100)
		avgPrice := 0.0
		if p.TotalVolumeLots > 0 {
			avgPrice = p.TotalValue / (p.TotalVolumeLots * 100)
		}

		sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, p.StockSymbol, p.Action))
		sb.WriteString(fmt.Sprintf("   - Intensitas: %d kali 'HAKA' dalam %.0f menit (Avg Interval: %.1f menit)\n", p.AlertCount, duration, duration/float64(p.AlertCount)))
		sb.WriteString(fmt.Sprintf("   - Aggregated Value: Rp %.2f Miliar\n", p.TotalValue/billionDivisor))
		sb.WriteString(fmt.Sprintf("   - Avg Price Estimation: %.0f\n", avgPrice))
		sb.WriteString(fmt.Sprintf("   - Kekuatan Anomali (Avg Z-Score): %.2f\n\n", p.AvgZScore))
	}

	sb.WriteString("Tugas Analisis (DATA DRIVEN):\n")
	sb.WriteString("1. **Identifikasi Fase**: Berdasarkan ritme pembelian (interval) dan volume, apakah ini Akumulasi rapi atau Hajar Kanan panik?\n")
	sb.WriteString("2. **Signifikansi Dana**: Apakah nilai total transaksi ini cukup besar untuk menggerakkan harga saham ini?\n")
	sb.WriteString("3. **Skenario Taktis**: Jika harga koreksi ke level average price, apakah itu 'Buy on Dip' yang valid?\n")
	sb.WriteString(fmt.Sprintf("\nJawab dalam Bahasa Indonesia yang tajam. Fokus pada angka. Maksimal %d kata.", maxPromptWords))

	return sb.String()
}

// FormatAnomalyPrompt creates a prompt for analyzing extreme Z-score events
func FormatAnomalyPrompt(anomalies []database.WhaleAlert) string {
	var sb strings.Builder
	sb.Grow(1024 + len(anomalies)*200)

	sb.WriteString("Anda mendeteksi anomali statistik ekstrem (Black Swan Event). Analisis mikrosturktur market berikut:\n\n")

	for i, a := range anomalies {
		if i >= maxAnomalies {
			break
		}

		zScore := safeFloat64(a.ZScore, 0.0)
		volPct := safeFloat64(a.VolumeVsAvgPct, 0.0)
		timeSince := time.Since(a.DetectedAt).Minutes()
		avgPrice := safeFloat64(a.AvgPrice, a.TriggerPrice)

		// Deviation calc
		devPct := 0.0
		if avgPrice > 0 {
			devPct = ((a.TriggerPrice - avgPrice) / avgPrice) * 100
		}

		sb.WriteString(fmt.Sprintf("%d. **%s** - %s Ekstrem!\n", i+1, a.StockSymbol, a.Action))
		sb.WriteString(fmt.Sprintf("   - Waktu: %.0f menit yang lalu\n", timeSince))
		sb.WriteString(fmt.Sprintf("   - Kekuatan Anomali (Z-Score): %.2f\n", zScore))
		sb.WriteString(fmt.Sprintf("   - Volume Spike: %.0f%% vs Rata-rata\n", volPct))
		sb.WriteString(fmt.Sprintf("   - Harga Eksekusi: %.0f (Deviasi dari Avg Price: %+.2f%%)\n", a.TriggerPrice, devPct))
		sb.WriteString(fmt.Sprintf("   - Nilai Transaksi: Rp %.2f Juta\n\n", a.TriggerValue/millionDivisor))
	}

	sb.WriteString("Analisis Forensik:\n")
	sb.WriteString("1. **Sifat Anomali**: Apakah deviasi harga dan Z-score menunjukkan 'Breakout' valid atau 'Fat Finger'?\n")
	sb.WriteString("2. **Psikologi Pelaku**: Adakah urgensi ekstrim (FOMO Buying / Panic Selling)?\n")
	sb.WriteString("3. **Rekomendasi**: Follow the flow atau Fade the move?\n")
	sb.WriteString("\nBerikan insight singkat & padat seolah-olah Anda adalah algoritmik trader.")

	return sb.String()
}

// FormatTimingPrompt creates a prompt for time-based pattern analysis
func FormatTimingPrompt(stats []database.TimeBasedStat) string {
	var sb strings.Builder
	sb.Grow(1024 + len(stats)*100)

	sb.WriteString("Analisis Time-Series Profiling dari aktivitas Smart Money:\n\n")

	for _, s := range stats {
		hour := s.TimeBucket
		netBuyVal := (s.TotalValue / float64(s.AlertCount)) // Rough avg value per alert

		sb.WriteString(fmt.Sprintf("🕒 **Jam %s:00**\n", hour))
		sb.WriteString(fmt.Sprintf("   - Aktivitas: %d alert (Beli: %d | Jual: %d)\n", s.AlertCount, s.BuyCount, s.SellCount))
		sb.WriteString(fmt.Sprintf("   - Total Perputaran Uang: Rp %.1f Miliar\n", s.TotalValue/billionDivisor))
		sb.WriteString(fmt.Sprintf("   - Avg Value per Alert: Rp %.1f Juta\n", netBuyVal/millionDivisor))
	}

	sb.WriteString("\nEvaluasi Strategis:\n")
	sb.WriteString("1. **Time Discovery**: Kapan 'Big Money' paling agresif? Apakah di Open, Mid-day, atau Close?\n")
	sb.WriteString("2. **Identifikasi Pola**: Adakah pola 'Morning Pump' atau 'Afternoon Dump'?\n")
	sb.WriteString("3. **Saran Eksekusi**: Kapan waktu terbaik bagi Retail untuk 'menumpang' arus ini?\n")
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
	sb.WriteString(fmt.Sprintf("Lakukan Bedah Saham (Stock Opname) untuk **%s** berdasarkan aliran dana Bandar (Whale Flow):\n\n", symbol))

	counts := countAlerts(alerts, false)

	// Metrics Calculation
	totalTrans := counts.buyCount + counts.sellCount + counts.unknownCount
	totalVal := counts.totalBuyValue + counts.totalSellValue + counts.totalUnknownValue

	buyRatio := 0.0
	if totalVal > 0 {
		buyRatio = (counts.totalBuyValue / totalVal) * 100
	}

	avgBuySize := 0.0
	if counts.buyCount > 0 {
		avgBuySize = counts.totalBuyValue / float64(counts.buyCount)
	}

	avgSellSize := 0.0
	if counts.sellCount > 0 {
		avgSellSize = counts.totalSellValue / float64(counts.sellCount)
	}

	sb.WriteString(fmt.Sprintf("📊 **Statistik Kunci (%d Data Terakhir)**:\n", totalTrans))
	sb.WriteString(fmt.Sprintf("- 🟢 **Bulls (Buy)**: %d ord | Rp %.2f M | Avg Size: Rp %.1f Juta\n", counts.buyCount, counts.totalBuyValue/billionDivisor, avgBuySize/millionDivisor))
	sb.WriteString(fmt.Sprintf("- 🔴 **Bears (Sell)**: %d ord | Rp %.2f M | Avg Size: Rp %.1f Juta\n", counts.sellCount, counts.totalSellValue/billionDivisor, avgSellSize/millionDivisor))
	sb.WriteString(fmt.Sprintf("- ⚖️ **Dominasi Buyer**: %.1f%%\n", buyRatio))

	if counts.unknownCount > 0 {
		sb.WriteString(fmt.Sprintf("- ⚪ **Netral/Crossing**: %d transaksi (Total Rp %.2f M)\n", counts.unknownCount, counts.totalUnknownValue/billionDivisor))
	}

	// Add trend context
	if buyRatio > 65 {
		sb.WriteString("\nKonteks: **Strong Accumulation** (>65% Flow is Buy).\n")
	} else if buyRatio < 35 {
		sb.WriteString("\nKonteks: **Strong Distribution** (<35% Flow is Buy).\n")
	} else {
		sb.WriteString("\nKonteks: **Consolidation / Battle** (Power seimbang).\n")
	}

	sb.WriteString("\nAnalisis Cepat (Micro-Structure):\n")
	sb.WriteString("1. Bandingkan 'Avg Size' Buy vs Sell. Apakah pembeli lebih 'berani' (ukuran order lebih besar)?\n")
	sb.WriteString("2. Prediksi jangka pendek berdasarkan dominasi flow?\n")
	sb.WriteString("3. Skor Potensi Kenaikan (1-10)?\n")
	sb.WriteString("Jawab <100 kata. Langsung pada inti.")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	return client.Analyze(ctx, sb.String())
}

// FormatSymbolAnalysisPrompt creates a detailed prompt for symbol-specific streaming analysis
func FormatSymbolAnalysisPrompt(symbol string, alerts []database.WhaleAlert) string {
	var sb strings.Builder
	sb.Grow(2048 + len(alerts)*50)

	sb.WriteString(fmt.Sprintf("Anda adalah AI Quantum Trader (Data-Driven Analyst). Lakukan Deep Dive pada **%s** berdasarkan data berikut:\n\n", symbol))

	if len(alerts) == 0 {
		sb.WriteString("Info: Tidak ada jejak aktivitas Big Player terdeteksi. Market sepi.\n")
		return sb.String()
	}

	counts := countAlerts(alerts, true)
	totalVal := counts.totalBuyValue + counts.totalSellValue + counts.totalUnknownValue

	buyPct := 0.0
	if totalVal > 0 {
		buyPct = (counts.totalBuyValue / totalVal) * 100
	}

	// Avg Trade Size
	avgBuy := 0.0
	if counts.buyCount > 0 {
		avgBuy = counts.totalBuyValue / float64(counts.buyCount)
	}

	avgSell := 0.0
	if counts.sellCount > 0 {
		avgSell = counts.totalSellValue / float64(counts.sellCount)
	}

	// Summary Statistic
	sb.WriteString(fmt.Sprintf("📊 **Peta Kekuatan Bandar (%d Transaksi)**:\n", len(alerts)))
	sb.WriteString(fmt.Sprintf("Total Volume: Rp %.1f Miliar\n", totalVal/billionDivisor))
	sb.WriteString(fmt.Sprintf("- 🐂 Buyer Power: Rp %.1f M (%.1f%%) | Avg Order: Rp %.0f Juta\n", counts.totalBuyValue/millionDivisor, buyPct, avgBuy/millionDivisor))
	sb.WriteString(fmt.Sprintf("- 🐻 Seller Power: Rp %.1f M (%.1f%%) | Avg Order: Rp %.0f Juta\n", counts.totalSellValue/millionDivisor, 100-buyPct, avgSell/millionDivisor))
	sb.WriteString("\n")

	// Whale Tracking
	sb.WriteString("🐋 **Whale Radar (Top Transactions)**:\n")
	if counts.maxBuyValue > 0 {
		sb.WriteString(fmt.Sprintf("✅ **Biggest Buy**: Rp %.1f M", counts.maxBuyValue/millionDivisor))
		if counts.maxBuyAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Z-Score: %.2f - %.0fx Avg Vol)", *counts.maxBuyAlert.ZScore, safeFloat64(counts.maxBuyAlert.VolumeVsAvgPct, 0)/100))
		}
		sb.WriteString("\n")
	}
	if counts.maxSellValue > 0 {
		sb.WriteString(fmt.Sprintf("❌ **Biggest Sell**: Rp %.1f M", counts.maxSellValue/millionDivisor))
		if counts.maxSellAlert.ZScore != nil {
			sb.WriteString(fmt.Sprintf(" (Z-Score: %.2f)", *counts.maxSellAlert.ZScore))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n**Executive Summary (DATA-DRIVEN ONLY)**:\n")
	sb.WriteString("1. **Market Structure**: Siapa yang mendominasi 'Average Order Size'? Jika Buyer > Seller, apakah ini akumulasi tersembunyi?\n")
	sb.WriteString("2. **Key Action**: Deteksi support nyata dari level harga transaksi terbesar.\n")
	sb.WriteString("3. **Final Verdict**: \n")
	sb.WriteString("   - **Signal**: AGGRESSIVE BUY / ACCUMULATION / WAIT / DISTRIBUTION\n")
	sb.WriteString("   - **Rationale**: Jelaskan alasan matematis berdasarkan flow di atas.\n")
	sb.WriteString(fmt.Sprintf("\nBerikan analisis mendalam, tajam, dan profesional. Dilarang berhalusinasi. Maksimal %d kata.", maxPromptWords))

	return sb.String()
}
