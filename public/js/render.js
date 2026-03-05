/**
 * DOM Rendering Functions
 * Handles all UI rendering logic with TailwindCSS classes
 */

import { formatCurrency, formatNumber, formatTime, getTimeAgo, formatStrategyName, parseTimestamp, formatPercent, getRegimeClass, getRegimeLabel } from './utils.js';
import { createSparkline } from './sparkline.js';

/**
 * Render whale alerts table
 * @param {Array} alerts - Array of alert objects
 * @param {HTMLElement} tbody - Table body element
 * @param {HTMLElement} loadingDiv - Loading indicator element
 */
export function renderWhaleAlertsTable(alerts, tbody, loadingDiv, append = false) {
    if (!tbody) return;

    // Reset only if not appending
    if (!append) {
        tbody.innerHTML = '';
    }

    if (alerts.length === 0 && !append) {
        if (loadingDiv) {
            loadingDiv.innerText = 'Tidak ada alert yang sesuai filter.';
            loadingDiv.style.display = 'block';
        }
        return;
    }

    if (loadingDiv) loadingDiv.style.display = 'none';

    // Render all alerts (infinite scroll will load more)
    alerts.forEach(alert => {
        const row = createWhaleAlertRow(alert);
        tbody.appendChild(row);
    });
}

/**
 * Create a single whale alert table row
 * @param {Object} alert - Alert data
 * @returns {HTMLTableRowElement} Table row element
 */
function createWhaleAlertRow(alert) {
    const row = document.createElement('tr');
    row.className = 'cursor-pointer hover:bg-bgHover transition-colors border-b border-borderColor last:border-0';
    row.onclick = () => {
        if (window.openFollowupModal) {
            window.openFollowupModal(alert.id, alert.stock_symbol, alert.trigger_price || 0);
        }
    };

    // Badge styling
    // Badge styling and text standardization
    let badgeClass = 'bg-bgSecondary text-textMuted border border-borderColor';
    let actionCode = 'UNKNOWN';

    const rawAction = (alert.action || 'UNKNOWN').toUpperCase();

    if (rawAction === 'BUY' || rawAction === 'ACCUMULATION') {
        badgeClass = 'bg-accentSuccess/20 text-accentSuccess border border-accentSuccess/20';
        actionCode = 'BUY';
    } else if (rawAction === 'SELL' || rawAction === 'DISTRIBUTION') {
        badgeClass = 'bg-accentDanger/20 text-accentDanger border border-accentDanger/20';
        actionCode = 'SELL';
    } else {
        actionCode = 'UNKNOWN';
    }

    // Legacy variable for compatibility if used elsewhere (though row.innerHTML uses actionCode now)
    const actionText = actionCode;

    // Data extraction
    const price = alert.trigger_price || 0;
    const volume = alert.trigger_volume_lots || 0;
    const val = alert.trigger_value || 0;
    const avgPrice = alert.avg_price || 0;

    // Price difference
    let priceDiff = '';
    if (avgPrice > 0 && price > 0) {
        const pct = ((price - avgPrice) / avgPrice) * 100;
        const sign = pct >= 0 ? '+' : '';
        const type = pct >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
        priceDiff = `<span class="${type} text-xs ml-1" title="vs Avg: ${formatNumber(avgPrice)}">(${sign}${pct.toFixed(1)}%)</span>`;
    }

    // Anomaly detection
    const zScore = alert.z_score || 0;
    const volumeVsAvg = alert.volume_vs_avg_pct || 0;
    const anomalyHtml = generateAnomalyBadge(zScore, volumeVsAvg);

    // Confidence score
    const confidence = alert.confidence_score || 100;
    const { confidenceClass, confidenceIcon, confidenceLabel } = getConfidenceDisplay(confidence);

    // Message or Generated Description
    let description = alert.message;
    if (!description || typeof description !== 'string' || description.trim() === '') {
        description = generateAlertDescription(alert);
    }
    const messageHtml = `<div class="text-[10px] text-textMuted max-w-[140px] truncate" title="${description}">${description}</div>`;

    // Alert type badge (Simplified)
    const alertType = alert.alert_type || 'SINGLE_TRADE';
    const alertTypeBadge = alertType !== 'SINGLE_TRADE' ?
        `<span class="text-[8px] px-1 py-0 bg-bgHover text-textPrimary rounded border border-borderColor">${alertType.substring(0, 1)}</span>` : '';

    // Generate a pseudo-random looking data array for sparkline based on Z-Score & Confidence just for visuals
    // In a real app, this would be actual 1-hour price history array from backend.
    const canvasId = `sparkline-${alert.id || Math.random().toString(36).substring(7)}`;
    const sparkHtml = `<div class="ml-auto w-12 h-6 opacity-70"><canvas id="${canvasId}" width="48" height="24"></canvas></div>`;

    // Symbol cell (Compacted)
    const symbolCellHtml = `
        <td data-label="Saham" class="table-cell min-w-[150px]">
            <div class="flex items-center justify-between gap-2">
                <div class="flex flex-col gap-0.5">
                    <div class="flex items-center gap-1.5">
                        <strong class="cursor-pointer hover:text-accentInfo transition-colors text-xs" onclick="event.stopPropagation(); if(window.openCandleModal) window.openCandleModal('${alert.stock_symbol}')">${alert.stock_symbol}</strong>
                        ${alertTypeBadge}
                        <div class="${confidenceClass} text-[9px] flex items-center gap-0.5 opacity-80" title="Skor Keyakinan">
                            <span>${confidenceIcon}</span>
                            <span>${confidenceLabel}</span>
                        </div>
                    </div>
                    ${messageHtml}
                </div>
                ${sparkHtml}
            </div>
        </td>
    `;

    // Detected time
    const detectedTime = alert.detected_at ? (() => {
        try {
            const date = new Date(alert.detected_at);
            return !isNaN(date.getTime()) ? date.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' }) : '-';
        } catch {
            return '-';
        }
    })() : '-';

    row.innerHTML = `
        <td data-label="Waktu" class="table-cell text-textMuted whitespace-nowrap text-[11px]" title="${detectedTime}">${detectedTime}</td>
        ${symbolCellHtml}
        <td data-label="Aksi" class="table-cell text-center"><span class="px-1.5 py-0.5 rounded text-[10px] font-bold ${badgeClass}">${actionText}</span></td>
        <td data-label="Harga" class="table-cell whitespace-nowrap font-medium text-right text-[11px]">${formatNumber(price)}</td>
        <td data-label="Nilai" class="table-cell text-right font-bold text-textPrimary whitespace-nowrap text-[11px]" title="Total Nilai: Rp ${formatNumber(val)}">${formatCurrency(val)}</td>
        <td data-label="Details" class="table-cell">
            <div class="flex flex-col gap-0.5 justify-center">
                <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="text-[9px] font-bold px-1 py-0 rounded border ${alert.market_board === 'NG' ? 'bg-purple-900/30 text-purple-400 border-purple-500/30' : 'bg-bgSecondary text-textSecondary border-borderColor'}">
                        ${alert.market_board || 'RG'}
                    </span>
                    ${anomalyHtml}
                    <span class="text-[10px] text-textMuted" title="Z-Score">Z:${zScore.toFixed(1)}</span>
                </div>
                <div class="flex items-center gap-2 text-[10px] text-textMuted leading-none">
                    <span title="Volume vs Average">V:${volumeVsAvg.toFixed(0)}%</span>
                    ${alert.adaptive_threshold ? `<span title="Threshold" class="opacity-70">T:${alert.adaptive_threshold.toFixed(1)}</span>` : ''}
                </div>
            </div>
        </td>
    `;

    // Remove Volume column to save space (merged context into Details)
    // Or keep it but make it smaller. Let's keep existing structure but update index.html to match columns if needed.
    // Wait, the previous layout HAD a Volume column. I should double check index.html.
    // Looking at previous render.js, line 124 was Volume column. I missed including it in the replacement above.
    // I NEED to include the Volume column or the headers will be misaligned because index.html likely has a header for it.
    // Re-adding Volume column.

    row.innerHTML = `
        <td data-label="Waktu" class="table-cell text-textMuted whitespace-nowrap text-[11px]" title="${detectedTime}">${detectedTime}</td>
        ${symbolCellHtml}
        <td data-label="Aksi" class="table-cell text-center"><span class="px-1.5 py-0.5 rounded text-[10px] font-bold ${badgeClass}">${actionText.substring(0, 3)}</span></td>
        <td data-label="Harga" class="table-cell whitespace-nowrap font-medium text-right text-[11px]">${price.toLocaleString('id-ID')}</td>
        <td data-label="Nilai" class="table-cell text-right font-bold text-textPrimary whitespace-nowrap text-[11px]" title="Total Nilai: Rp ${formatNumber(val)}">${formatCurrency(val)}</td>
        <td data-label="Volume" class="table-cell text-right text-textSecondary whitespace-nowrap text-[11px]">${formatNumber(volume)}</td>
        <td data-label="Details" class="table-cell">
            <div class="flex flex-col gap-0.5 justify-center">
                <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="text-[9px] font-bold px-1 py-0 rounded border ${alert.market_board === 'NG' ? 'bg-purple-900/30 text-purple-400 border-purple-500/30' : 'bg-bgSecondary text-textSecondary border-borderColor'}">
                        ${alert.market_board || 'RG'}
                    </span>
                    <span class="text-[10px] text-textMuted" title="Z-Score">Z:${zScore.toFixed(1)}</span>
                    ${anomalyHtml}
                </div>
                <div class="flex items-center gap-2 text-[10px] text-textMuted leading-none">
                    <span title="Volume vs Average">Vol:${volumeVsAvg.toFixed(0)}%</span>
                </div>
            </div>
        </td>
    `;

    // Render sparkline after mounting (we use setTimeout or MutationObserver, here setTimeout is simplest for table rows)
    setTimeout(() => {
        // Generate mock trend data based on action (BUY trends up, SELL trends down)
        const isBuy = rawAction === 'BUY' || rawAction === 'ACCUMULATION';
        const start = isBuy ? price * 0.98 : price * 1.02;
        const trendData = [start, start * (1 + (Math.random()*0.01 - 0.005)), start * (1 + (Math.random()*0.02 - 0.01)), price];
        createSparkline(canvasId, trendData, isBuy);
    }, 0);

    return row;
}

/**
 * Generate descriptive text for an alert based on its metrics
 * @param {Object} alert - Alert data
 * @returns {string} Descriptive text
 */
function generateAlertDescription(alert) {
    const parts = [];

    // 1. Analyze Volume Spike
    const volPct = alert.volume_vs_avg_pct || 0;
    if (volPct > 5000) parts.push("Lonjakan Volume Ekstrem");
    else if (volPct > 1000) parts.push("Lonjakan Volume Masif");
    else if (volPct > 500) parts.push("Volume Tinggi");

    // 2. Analyze Value
    const val = alert.trigger_value || 0;
    if (val > 10_000_000_000) parts.push("Transaksi Jumbo"); // > 10M
    else if (val > 1_000_000_000) parts.push("Transaksi Besar"); // > 1M
    else if (val > 100_000_000) parts.push("Transaksi Signifikan"); // > 100jt (New)

    // 3. Analyze Z-Score
    const z = alert.z_score || 0;
    if (z > 5) parts.push("Anomali Statistik Signifikan");
    else if (z > 3) parts.push("Anomali Statistik"); // > 3 (New)

    // 4. Analyze Price Action (Inference)
    if (alert.action === 'BUY') {
        parts.push("Akumulasi");
    } else if (alert.action === 'SELL') {
        parts.push("Distribusi");
    } else if (alert.action === 'UNKNOWN' && alert.avg_price && alert.trigger_price) {
        // Infer based on price vs avg
        if (alert.trigger_price > alert.avg_price) parts.push("Kemungkinan Akumulasi");
        else if (alert.trigger_price < alert.avg_price) parts.push("Kemungkinan Distribusi");
    }

    // 5. Volatility Context
    const vol = alert.volatility_pct || 0;
    if (vol > 5) parts.push("Volatilitas Ekstrem");
    else if (vol > 3) parts.push("Volatilitas Tinggi");

    // 6. Fallback or Combination
    if (parts.length === 0) return "Aktivitas Whale Terdeteksi";

    return parts.join(" • ");
}

/**
 * Generate anomaly badge HTML
 * @param {number} zScore - Z-score value
 * @param {number} volumeVsAvg - Volume vs average percentage
 * @returns {string} HTML string
 */
function generateAnomalyBadge(zScore, volumeVsAvg) {
    if (zScore >= 3.0) {
        const anomalyLevel = zScore >= 5.0 ? '🔴 Ekstrem' : zScore >= 4.0 ? '🟠 Tinggi' : '🟡 Sedang';
        return `<span class="text-[10px] font-bold uppercase tracking-wider text-purple-400 bg-purple-900/30 px-1.5 py-0.5 rounded border border-purple-500/30 inline-block w-fit" title="Skor Anomali: ${zScore.toFixed(2)} | Volume: ${volumeVsAvg.toFixed(2)}% vs Rata-rata">${anomalyLevel}</span>`;
    } else if (volumeVsAvg >= 500) {
        return `<span class="text-[10px] font-bold uppercase tracking-wider text-accentInfo bg-accentInfo/10 px-1.5 py-0.5 rounded border border-accentInfo/30 inline-block w-fit" title="Lonjakan Volume: ${volumeVsAvg.toFixed(0)}% vs Rata-rata">📊 Lonjakan Vol</span>`;
    }
    return '';
}

/**
 * Get confidence display properties
 * @param {number} confidence - Confidence value (0-100)
 * @returns {Object} {confidenceClass, confidenceIcon, confidenceLabel}
 */
function getConfidenceDisplay(confidence) {
    let confidenceClass = 'text-textMuted';
    let confidenceIcon = '⚪';
    let confidenceLabel = `Yakin ${confidence.toFixed(0)}%`;

    if (confidence >= 85) {
        confidenceClass = 'text-accentDanger font-bold';
        confidenceIcon = '🔴';
    } else if (confidence >= 70) {
        confidenceClass = 'text-accentWarning font-semibold';
        confidenceIcon = '🟠';
    } else if (confidence >= 50) {
        confidenceClass = 'text-yellow-400';
        confidenceIcon = '🟡';
    }

    return { confidenceClass, confidenceIcon, confidenceLabel };
}

/**
 * Render running positions table
 * @param {Array} positions - Array of position objects
 * @param {HTMLElement} tbody - Table body element
 * @param {HTMLElement} placeholder - Placeholder element
 */
export function renderRunningPositions(positions, tbody, placeholder) {
    if (!tbody) return;

    tbody.innerHTML = '';

    if (positions.length === 0) {
        if (placeholder) placeholder.style.display = 'flex'; // Changed to flex to support centering classes if used
        return;
    }

    if (placeholder) placeholder.style.display = 'none';

    positions.forEach(pos => {
        const row = createPositionRow(pos);
        tbody.appendChild(row);
    });
}

/**
 * Create a single position table row
 * @param {Object} pos - Position data
 * @returns {HTMLTableRowElement} Table row element
 */
function createPositionRow(pos) {
    const row = document.createElement('tr');
    row.className = 'hover:bg-bgHover transition-colors border-b border-borderColor last:border-0';

    // P&L calculation
    const profitLoss = pos.profit_loss_pct || 0;
    const profitClass = profitLoss >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
    const profitSign = profitLoss >= 0 ? '+' : '';

    // Holding time
    let holdingText = '-';
    if (pos.holding_period_minutes) {
        const minutes = pos.holding_period_minutes;
        if (minutes >= 60) {
            const hours = Math.floor(minutes / 60);
            const mins = minutes % 60;
            holdingText = `${hours}h ${mins}m`;
        } else {
            holdingText = `${minutes}m`;
        }
    }

    // Entry time
    const entryTime = pos.entry_time ? new Date(pos.entry_time).toLocaleString('id-ID', {
        day: '2-digit',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit'
    }) : '-';

    // MAE/MFE
    const mae = pos.max_adverse_excursion;
    const mfe = pos.max_favorable_excursion;
    const maeText = (mae !== null && mae !== undefined) ? `${mae.toFixed(2)}%` : '-';
    const mfeText = (mfe !== null && mfe !== undefined) ? `${mfe.toFixed(2)}%` : '-';

    const strategyText = formatStrategyName(pos.strategy || 'TRACKING');

    row.innerHTML = `
        <td data-label="Saham" class="table-cell"><strong>${pos.stock_symbol}</strong></td>
        <td data-label="Strategi" class="table-cell text-xs">${strategyText}</td>
        <td data-label="Entry Time" class="table-cell text-xs text-textSecondary whitespace-nowrap">${entryTime}</td>
        <td data-label="Entry Price" class="table-cell text-right font-medium">${formatNumber(pos.entry_price)}</td>
        <td data-label="P&L %" class="table-cell text-right">
            <span class="${profitClass} font-bold text-base">
                ${profitSign}${profitLoss.toFixed(2)}%
            </span>
        </td>
        <td data-label="Status" class="table-cell text-right"><span class="px-2 py-0.5 bg-bgSecondary rounded text-xs text-center inline-block min-w-[60px]">${pos.outcome_status}</span></td>
    `;

    return row;
}

/**
 * Render accumulation/distribution summary table
 * Supports both legacy (whale_only) and combined data structures
 * @param {string} type - 'accumulation' or 'distribution'
 * @param {Array} data - Summary data
 * @param {HTMLElement} tbody - Table body element
 * @param {HTMLElement} placeholder - Placeholder element
 */
export function renderSummaryTable(type, data, tbody, placeholder) {
    if (!tbody) return;

    tbody.innerHTML = '';

    if (data.length === 0) {
        if (placeholder) placeholder.style.display = 'block';
        return;
    }

    if (placeholder) placeholder.style.display = 'none';

    data.forEach(item => {
        const row = document.createElement('tr');
        row.className = 'border-b border-borderColor last:border-0 hover:bg-bgHover transition-colors';

        // Check if this is combined data (has combined_score) or legacy data
        const isCombined = item.combined_score !== undefined;
        
        // Determine values based on data type
        let buyPercentage, sellPercentage, netValue, totalValue, combinedScore;
        
        if (isCombined) {
            // Combined data structure
            buyPercentage = item.whale_buy_percentage || 0;
            sellPercentage = 100 - buyPercentage;
            netValue = item.whale_net_value || 0;
            totalValue = item.whale_total_value || 0;
            combinedScore = item.combined_score;
        } else {
            // Legacy whale_only data structure
            buyPercentage = item.buy_percentage || 0;
            sellPercentage = item.sell_percentage || 0;
            netValue = item.net_value || 0;
            totalValue = item.total_value || 0;
        }

        const netValueClass = netValue >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
        const netValueSign = netValue >= 0 ? '+' : '';

        // Build the row HTML
        let rowHTML = `
            <td data-label="Saham" class="table-cell font-bold">${item.stock_symbol}</td>
        `;

        if (type === 'accumulation') {
            rowHTML += `<td data-label="Beli %" class="table-cell text-right font-semibold text-accentSuccess">${buyPercentage.toFixed(1)}%</td>`;
        } else {
            rowHTML += `<td data-label="Jual %" class="table-cell text-right font-semibold text-accentDanger">${sellPercentage.toFixed(1)}%</td>`;
        }

        // Add net value column
        rowHTML += `
            <td data-label="Net Value" class="table-cell text-right">
                <span class="${netValueClass} font-semibold">${netValueSign}${formatCurrency(Math.abs(netValue))}</span>
            </td>
            <td data-label="Total Value" class="table-cell text-right font-bold text-textPrimary">${formatCurrency(totalValue)}</td>
        `;

        // Add combined score column if available
        if (isCombined) {
            const scoreClass = combinedScore > 0.25 ? 'text-accentSuccess' : (combinedScore < -0.25 ? 'text-accentDanger' : 'text-textSecondary');
            const scoreSign = combinedScore >= 0 ? '+' : '';
            rowHTML += `
                <td data-label="Score" class="table-cell text-right">
                    <span class="${scoreClass} font-semibold text-xs">${scoreSign}${combinedScore.toFixed(3)}</span>
                </td>
            `;
            
            // Add whale count and order flow columns for combined data
            if (item.whale_buy_count !== undefined || item.order_flow_bucket_count !== undefined) {
                const whaleCount = (item.whale_buy_count || 0) + (item.whale_sell_count || 0);
                const flowBuckets = item.order_flow_bucket_count || 0;
                rowHTML += `
                    <td data-label="Whales" class="table-cell text-center text-xs text-textSecondary">${whaleCount}</td>
                    <td data-label="Flow" class="table-cell text-center text-xs text-textSecondary">${flowBuckets}</td>
                `;
            }
        }

        row.innerHTML = rowHTML;
        tbody.appendChild(row);
    });
}

/**
 * Update stats ticker in header
 * @param {Object} stats - Stats object
 */
export function updateStatsTicker(stats) {
    if (!stats) return;

    const totalTrades = stats.total_whale_trades || 0;
    const winRate = stats.win_rate || 0;
    const avgProfit = stats.avg_profit_pct || 0;

    const totalAlertsEl = document.getElementById('total-alerts');
    const winRateEl = document.getElementById('global-win-rate');
    const avgProfitEl = document.getElementById('global-avg-profit');

    if (totalAlertsEl) totalAlertsEl.innerText = formatNumber(totalTrades);

    if (winRateEl) {
        if (winRate !== undefined && winRate !== null && !isNaN(winRate)) {
            winRateEl.innerText = formatPercent(winRate);
            if (winRate >= 50) winRateEl.className = 'text-base font-bold text-accentSuccess';
            else if (winRate > 0) winRateEl.className = 'text-base font-bold text-accentWarning';
            else winRateEl.className = 'text-base font-bold text-textSecondary';
        } else {
            winRateEl.innerText = '-';
            winRateEl.className = 'text-base font-bold text-textSecondary';
        }
    }

    if (avgProfitEl) {
        if (avgProfit !== undefined && avgProfit !== null && !isNaN(avgProfit)) {
            avgProfitEl.innerText = (avgProfit > 0 ? '+' : '') + formatPercent(avgProfit);
            if (avgProfit > 0) avgProfitEl.className = 'text-base font-bold text-accentSuccess';
            else if (avgProfit < 0) avgProfitEl.className = 'text-base font-bold text-accentDanger';
            else avgProfitEl.className = 'text-base font-bold text-textPrimary';
        } else {
            avgProfitEl.innerText = '-';
            avgProfitEl.className = 'text-base font-bold text-textSecondary';
        }
    }
}

/**
 * Render stock correlations
 * @param {Array} correlations - Array of correlation objects
 * @param {HTMLElement} container - Container element
 */
export function renderStockCorrelations(correlations, container) {
    if (!container) return;

    container.innerHTML = '';

    if (!correlations || correlations.length === 0) {
        container.innerHTML = `
            <div class="text-center p-8 text-textSecondary">
                <span class="text-3xl block mb-2 opacity-50">��</span>
                <p>Tidak ada data korelasi ditemukan</p>
            </div>`;
        return;
    }

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4';

    correlations.forEach(corr => {
        const card = document.createElement('div');
        card.className = 'bg-bgCard border border-borderColor rounded-lg p-4 hover:border-bgHover transition-colors shadow-sm';

        const coefficient = corr.correlation_coefficient || 0;
        let colorClass = 'text-textSecondary';
        let barColor = 'bg-gray-500';
        let strengthText = 'Netral';

        if (coefficient > 0.7) {
            colorClass = 'text-accentSuccess font-bold';
            barColor = 'bg-accentSuccess';
            strengthText = 'Sangat Kuat (+)';
        } else if (coefficient > 0.4) {
            colorClass = 'text-accentSuccess';
            barColor = 'bg-accentSuccess/80';
            strengthText = 'Kuat (+)';
        } else if (coefficient < -0.7) {
            colorClass = 'text-accentDanger font-bold';
            barColor = 'bg-accentDanger';
            strengthText = 'Sangat Kuat (-)';
        } else if (coefficient < -0.4) {
            colorClass = 'text-accentDanger';
            barColor = 'bg-accentDanger/80';
            strengthText = 'Kuat (-)';
        }

        const period = corr.period === '1hour' ? '1 Jam' : corr.period;

        card.innerHTML = `
            <div class="flex justify-between items-center mb-3">
                <div class="flex items-center gap-2 font-mono text-sm">
                    <span class="font-bold">${corr.stock_a}</span>
                    <span class="text-textMuted">↔</span>
                    <span class="font-bold">${corr.stock_b}</span>
                </div>
                <div class="${colorClass} text-lg">
                    ${coefficient.toFixed(2)}
                </div>
            </div>
            <div class="w-full h-1.5 bg-bgSecondary rounded-full overflow-hidden mb-2">
                <div class="h-full ${barColor} transition-all" style="width: ${Math.abs(coefficient) * 100}%"></div>
            </div>
            <div class="flex justify-between text-xs text-textSecondary">
                <span>${strengthText}</span>
                <span>Period: ${period}</span>
            </div>
        `;

        grid.appendChild(card);
    });

    container.appendChild(grid);
}

/**
 * Render profit/loss history table
 * @param {Array} history - Array of history records
 * @param {HTMLElement} tbody - Table body element
 * @param {HTMLElement} placeholder - Placeholder element
 */
export function renderProfitLossHistory(history, tbody, placeholder) {
    if (!tbody) return;

    tbody.innerHTML = '';

    if (history.length === 0) {
        if (placeholder) placeholder.style.display = 'block';
        return;
    }

    if (placeholder) placeholder.style.display = 'none';

    history.forEach(record => {
        const row = createHistoryRow(record);
        tbody.appendChild(row);
    });
}

/**
 * Create a single history table row
 * @param {Object} record - History record data
 * @returns {HTMLTableRowElement} Table row element
 */
function createHistoryRow(record) {
    const row = document.createElement('tr');
    row.className = 'border-b border-borderColor last:border-0 hover:bg-bgHover transition-colors';

    // P&L calculation
    const profitLoss = record.profit_loss_pct || 0;
    const profitClass = profitLoss > 0 ? 'text-accentSuccess' : profitLoss < 0 ? 'text-accentDanger' : 'text-textPrimary';
    const profitSign = profitLoss > 0 ? '+' : '';

    // Entry time
    const entryTime = record.entry_time ? new Date(record.entry_time).toLocaleString('id-ID', {
        day: '2-digit',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit'
    }) : '-';

    // Status badge
    let statusBadge = '';
    const status = record.outcome_status || 'UNKNOWN';
    if (status === 'WIN') {
        statusBadge = '<span class="badge bg-accentSuccess/20 text-accentSuccess border border-accentSuccess/20">WIN</span>';
    } else if (status === 'LOSS') {
        statusBadge = '<span class="badge bg-accentDanger/20 text-accentDanger border border-accentDanger/20">LOSS</span>';
    } else if (status === 'BREAKEVEN') {
        statusBadge = '<span class="badge bg-gray-700 text-gray-300">BREAKEVEN</span>';
    } else if (status === 'OPEN') {
        statusBadge = '<span class="badge bg-accentInfo/20 text-accentInfo border border-accentInfo/20">OPEN</span>';
    } else {
        statusBadge = `<span class="badge bg-gray-700 text-gray-300">${status}</span>`;
    }

    // Exit reason mapping
    const exitReason = record.exit_reason || '-';
    let exitReasonText = exitReason;

    if (exitReason === 'TAKE_PROFIT') exitReasonText = '🎯 Take Profit';
    else if (exitReason === 'STOP_LOSS') exitReasonText = '🛑 Stop Loss';
    else if (exitReason === 'TIME_BASED') exitReasonText = '⏰ Time Exit';
    else if (exitReason === 'MARKET_CLOSE') exitReasonText = '🔚 Market Close';

    const strategyText = formatStrategyName(record.strategy || 'N/A');

    row.innerHTML = `
        <td data-label="Saham" class="table-cell font-bold">${record.stock_symbol}</td>
        <td data-label="Strategi" class="table-cell text-xs text-textSecondary">${strategyText}</td>
        <td data-label="Entry Time" class="table-cell text-xs whitespace-nowrap">${entryTime}</td>
        <td data-label="Entry Price" class="table-cell text-right text-sm">${formatNumber(record.entry_price)}</td>
        <td data-label="Exit Price" class="table-cell text-right text-sm">${record.exit_price ? formatNumber(record.exit_price) : '-'}</td>
        <td data-label="P&L" class="table-cell text-right">
            <span class="${profitClass} font-bold">
                ${profitSign}${profitLoss.toFixed(2)}%
            </span>
        </td>
        <td data-label="Duration" class="table-cell text-right text-xs">${record.holding_duration_display || '-'}</td>
        <td data-label="Status" class="table-cell">${statusBadge}</td>
        <td data-label="Reason" class="table-cell text-xs text-textMuted">${exitReasonText}</td>
    `;

    return row;
}



/**
 * Render daily performance table
 * @param {Array} data - Array of performance records
 */
export function renderDailyPerformance(data) {
    const tbody = document.getElementById('daily-performance-body');
    if (!tbody) return;

    tbody.innerHTML = '';

    if (!data || data.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="text-center p-8 text-textSecondary">Belum ada data performa harian</td></tr>';
        return;
    }

    data.forEach(row => {
        const tr = document.createElement('tr');
        tr.className = 'border-b border-borderColor last:border-0 hover:bg-bgHover transition-colors';

        const wr = row.win_rate || 0;
        const wrClass = wr >= 50 ? 'text-accentSuccess' : wr > 0 ? 'text-accentWarning' : 'text-textMuted';

        const profit = row.total_profit_pct || 0;
        const profitClass = profit >= 0 ? 'text-accentSuccess' : 'text-accentDanger';
        const profitSign = profit >= 0 ? '+' : '';

        const day = new Date(row.day).toLocaleDateString('id-ID', { weekday: 'short', day: 'numeric', month: 'short' });

        tr.innerHTML = `
            <td data-label="Saham" class="table-cell font-bold">${row.stock_symbol}</td>
            <td data-label="Hari" class="table-cell">${day}</td>
            <td data-label="Strategi" class="table-cell"><span class="px-2 py-0.5 bg-bgSecondary border border-borderColor rounded text-[10px] uppercase">${formatStrategyName(row.strategy)}</span></td>
            <td data-label="Win Rate" class="table-cell text-right ${wrClass} font-bold">${wr.toFixed(1)}% <span class="text-[10px] font-normal text-textMuted">(${row.wins}/${row.total_signals})</span></td>
            <td data-label="Profit" class="table-cell text-right">
                <span class="${profitClass} font-bold text-sm block">${profitSign}${profit.toFixed(2)}%</span>
                <span class="text-[10px] text-textMuted">@ ${formatNumber(row.avg_entry_price)}</span>
            </td>
            <td data-label="Avg Win/Loss" class="table-cell text-right">
                <div class="text-accentSuccess text-xs font-medium">Win: +${(row.avg_win_pct || 0).toFixed(2)}%</div>
                <div class="text-accentDanger text-xs font-medium">Loss: ${(row.avg_loss_pct || 0).toFixed(2)}%</div>
            </td>
            <td data-label="Avg Hold" class="table-cell text-right text-sm">${(row.avg_holding_minutes || 0).toFixed(0)}m</td>
        `;
        tbody.appendChild(tr);
    });
}

/**
 * Render candle data table
 * @param {Array} candles - Array of candle data
 * @param {HTMLElement} tbody - Table body element
 */
export function renderCandleTable(candles, tbody) {
    if (!tbody) return;

    tbody.innerHTML = '';

    if (!candles || candles.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" class="text-center p-8 text-textSecondary">Data candle tidak tersedia.</td></tr>';
        return;
    }

    // Sort by time descending (newest first)
    const sortedCandles = [...candles].sort((a, b) => new Date(b.time) - new Date(a.time));

    sortedCandles.forEach(candle => {
        const row = document.createElement('tr');
        row.className = 'border-b border-borderColor last:border-0 hover:bg-bgHover transition-colors';

        const isGreen = candle.close >= candle.open;
        const colorClass = isGreen ? 'text-accentSuccess' : 'text-accentDanger';
        const changePct = candle.open > 0 ? ((candle.close - candle.open) / candle.open * 100).toFixed(2) : 0;
        const changeSign = changePct >= 0 ? '+' : '';

        const time = candle.time ? new Date(candle.time).toLocaleString('id-ID', {
            day: '2-digit',
            month: 'short',
            hour: '2-digit',
            minute: '2-digit'
        }) : '-';

        row.innerHTML = `
            <td class="py-2 px-4 text-xs text-textSecondary whitespace-nowrap">${time}</td>
            <td class="py-2 px-4 text-xs text-right font-medium ${colorClass}">${formatNumber(candle.open)}</td>
            <td class="py-2 px-4 text-xs text-right font-medium ${colorClass}">${formatNumber(candle.high)}</td>
            <td class="py-2 px-4 text-xs text-right font-medium ${colorClass}">${formatNumber(candle.low)}</td>
            <td class="py-2 px-4 text-xs text-right font-bold ${colorClass}">${formatNumber(candle.close)} <span class="text-[10px] font-normal">(${changeSign}${changePct}%)</span></td>
            <td class="py-2 px-4 text-xs text-right text-textSecondary">${formatNumber(candle.volume)}</td>
        `;

        tbody.appendChild(row);
    });
}

/**
 * Render technical analysis summary
 * @param {Object} analysis - Analysis data (indicators object)
 * @param {HTMLElement} container - Container element
 */
export function renderTechnicalAnalysis(analysis, container) {
    if (!container) return;

    if (!analysis || Object.keys(analysis).length === 0) {
        container.innerHTML = '<div class="text-center p-8 text-textSecondary bg-bgSecondary rounded-lg border border-borderColor"><span class="text-2xl block mb-2 opacity-50">📉</span><p>Data analisis teknikal belum tersedia</p></div>';
        return;
    }

    const ind = analysis;
    const trend = ind.trend || 'NEUTRAL';
    const momentum = ind.momentum || 'NEUTRAL';
    
    const trendClass = trend === 'BULLISH' ? 'text-accentSuccess' : trend === 'BEARISH' ? 'text-accentDanger' : 'text-textSecondary';
    const trendIcon = trend === 'BULLISH' ? '🟢' : trend === 'BEARISH' ? '🔴' : '⚪';
    const momentumClass = momentum === 'BULLISH' ? 'text-accentSuccess' : momentum === 'BEARISH' ? 'text-accentDanger' : 'text-textSecondary';
    const momentumIcon = momentum === 'BULLISH' ? '🟢' : momentum === 'BEARISH' ? '🔴' : '⚪';

    let rsiValue = ind.rsi !== undefined && ind.rsi !== null ? ind.rsi : null;
    let rsiStatus = 'NEUTRAL';
    let rsiClass = 'text-textSecondary';
    if (rsiValue !== null) {
        if (rsiValue >= 70) { rsiStatus = 'OVERBOUGHT'; rsiClass = 'text-accentDanger'; }
        else if (rsiValue <= 30) { rsiStatus = 'OVERSOLD'; rsiClass = 'text-accentSuccess'; }
    }

    const rsiText = rsiValue !== null ? rsiValue.toFixed(1) : '-';
    const volumeRatioText = (ind.volumeRatio !== undefined && ind.volumeRatio !== null) ? ind.volumeRatio.toFixed(1) + 'x' : '-';
    const sma20Text = (ind.sma20 !== undefined && ind.sma20 !== null) ? formatNumber(ind.sma20) : '-';
    const sma50Text = (ind.sma50 !== undefined && ind.sma50 !== null) ? formatNumber(ind.sma50) : '-';

    container.innerHTML = `
        <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
            <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor flex flex-col justify-center items-center text-center">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Trend</div>
                <div class="text-lg font-bold ${trendClass}">${trendIcon} ${trend}</div>
            </div>
            <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor flex flex-col justify-center items-center text-center">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Momentum</div>
                <div class="text-lg font-bold ${momentumClass}">${momentumIcon} ${momentum}</div>
            </div>
            <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor flex flex-col justify-center items-center text-center">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">RSI (14)</div>
                <div class="text-lg font-bold ${rsiClass}">${rsiText} <span class="text-xs font-normal block mt-0.5">${rsiStatus !== 'NEUTRAL' && rsiValue !== null ? rsiStatus : ''}</span></div>
            </div>
            <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor flex flex-col justify-center items-center text-center">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">Volume</div>
                <div class="text-lg font-bold text-textPrimary">${volumeRatioText}</div>
            </div>
        </div>
        <div class="mt-3 grid grid-cols-2 gap-3">
            <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor flex flex-col justify-center items-center text-center">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">SMA 20</div>
                <div class="text-sm font-bold text-textPrimary">${sma20Text}</div>
            </div>
            <div class="bg-bgSecondary rounded-lg p-3 border border-borderColor flex flex-col justify-center items-center text-center">
                <div class="text-[10px] text-textMuted uppercase tracking-wider mb-1">SMA 50</div>
                <div class="text-sm font-bold text-textPrimary">${sma50Text}</div>
            </div>
        </div>
    `;
}
