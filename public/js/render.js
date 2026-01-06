/**
 * DOM Rendering Functions
 * Handles all UI rendering logic
 */

import { formatCurrency, formatNumber, formatTime, getTimeAgo, formatStrategyName, parseTimestamp, formatPercent } from './utils.js';

/**
 * Render whale alerts table
 * @param {Array} alerts - Array of alert objects
 * @param {HTMLElement} tbody - Table body element
 * @param {HTMLElement} loadingDiv - Loading indicator element
 */
export function renderWhaleAlerts(alerts, tbody, loadingDiv) {
    if (!tbody) return;

    // Reset
    tbody.innerHTML = '';

    if (alerts.length === 0) {
        if (loadingDiv) {
            loadingDiv.innerText = 'Tidak ada alert yang sesuai filter.';
            loadingDiv.style.display = 'block';
        }
        return;
    }

    if (loadingDiv) loadingDiv.style.display = 'none';

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
    row.className = 'clickable-row';
    row.onclick = () => {
        if (window.openFollowupModal) {
            window.openFollowupModal(alert.id, alert.stock_symbol, alert.trigger_price || 0);
        }
    };

    // Badge styling
    let badgeClass = 'unknown';
    if (alert.action === 'BUY') badgeClass = 'buy';
    if (alert.action === 'SELL') badgeClass = 'sell';
    const actionText = alert.action === 'BUY' ? 'BELI' : alert.action === 'SELL' ? 'JUAL' : alert.action;

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
        const type = pct >= 0 ? 'diff-positive' : 'diff-negative';
        priceDiff = `<span class="${type}" title="vs Avg: ${formatNumber(avgPrice)}">(${sign}${pct.toFixed(1)}%)</span>`;
    }

    // Anomaly detection
    const zScore = alert.z_score || 0;
    const volumeVsAvg = alert.volume_vs_avg_pct || 0;
    const anomalyHtml = generateAnomalyBadge(zScore, volumeVsAvg);

    // Confidence score
    const confidence = alert.confidence_score || 100;
    const { confidenceClass, confidenceIcon, confidenceLabel } = getConfidenceDisplay(confidence);

    // Message
    const messageHtml = alert.message ?
        `<div style="font-size: 0.7rem; color: #555; margin-top: 4px; max-width: 200px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;" title="${alert.message}">${alert.message}</div>` : '';

    // Alert type badge
    const alertType = alert.alert_type || 'SINGLE_TRADE';
    const alertTypeBadge = alertType !== 'SINGLE_TRADE' ?
        `<span style="font-size:0.65em; padding:2px 4px; background:#333; color:#fff; border-radius:3px; margin-left:4px;">${alertType}</span>` : '';

    // Symbol cell
    const symbolCellHtml = `
        <td data-label="Saham" class="col-symbol">
            <div style="display: flex; align-items: center; gap: 4px;">
                <strong class="clickable-symbol" onclick="event.stopPropagation(); if(window.openCandleModal) window.openCandleModal('${alert.stock_symbol}')">${alert.stock_symbol}</strong>
                ${alertTypeBadge}
            </div>
            <span class="${confidenceClass}" style="font-size:0.7em;" title="Skor Keyakinan">${confidenceIcon} ${confidenceLabel}</span>
            ${messageHtml}
        </td>
    `;

    // Detected time
    const detectedTime = alert.detected_at ? (() => {
        try {
            const date = new Date(alert.detected_at);
            return !isNaN(date.getTime()) ? date.toLocaleString('id-ID') : 'Waktu tidak valid';
        } catch {
            return 'Waktu tidak valid';
        }
    })() : 'Waktu tidak valid';

    row.innerHTML = `
        <td data-label="Waktu" class="col-time" title="${detectedTime}">${formatTime(alert.detected_at)}</td>
        ${symbolCellHtml}
        <td data-label="Aksi"><span class="badge ${badgeClass}">${actionText}</span></td>
        <td data-label="Harga" class="col-price">${formatNumber(price)} ${priceDiff}</td>
        <td data-label="Nilai" class="text-right value-highlight" title="Total Nilai: Rp ${formatNumber(val)}">${formatCurrency(val)}</td>
        <td data-label="Volume" class="text-right" title="${formatNumber(volume)} lot">${formatNumber(volume)} Lot</td>
        <td data-label="Details">
            <div style="display: flex; flex-direction: column; gap: 2px;">
                <span style="font-size:0.85em; color:var(--text-secondary);">${alert.market_board || 'RG'}</span>
                ${anomalyHtml}
                ${!anomalyHtml ? `<span style="font-size:0.75em; color:#aaa;">${alertType === 'ACCUMULATION' ? 'Akumulasi' : 'Transaksi Besar'}</span>` : ''}
                ${zScore > 0 ? `<span style="font-size:0.7em; color:#888;" title="Statistical Anomaly Score">Z: ${zScore.toFixed(2)}</span>` : ''}
                <span style="font-size: 0.65em; color: var(--accent-blue); margin-top: 2px;">Klik info ↗</span>
            </div>
        </td>
    `;

    return row;
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
        return `<span class="table-anomaly" title="Skor Anomali: ${zScore.toFixed(2)} | Volume: ${volumeVsAvg.toFixed(0)}% vs Rata-rata">${anomalyLevel}</span>`;
    } else if (volumeVsAvg >= 500) {
        return `<span class="table-anomaly" title="Lonjakan Volume: ${volumeVsAvg.toFixed(0)}% vs Rata-rata">📊 Lonjakan Vol</span>`;
    }
    return '';
}

/**
 * Get confidence display properties
 * @param {number} confidence - Confidence value (0-100)
 * @returns {Object} {confidenceClass, confidenceIcon, confidenceLabel}
 */
function getConfidenceDisplay(confidence) {
    let confidenceClass = 'confidence-low';
    let confidenceIcon = '⚪';
    let confidenceLabel = `Yakin ${confidence.toFixed(0)}%`;

    if (confidence >= 85) {
        confidenceClass = 'confidence-extreme';
        confidenceIcon = '🔴';
    } else if (confidence >= 70) {
        confidenceClass = 'confidence-high';
        confidenceIcon = '🟠';
    } else if (confidence >= 50) {
        confidenceClass = 'confidence-medium';
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
        if (placeholder) placeholder.style.display = 'block';
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

    // P&L calculation
    const profitLoss = pos.profit_loss_pct || 0;
    const profitClass = profitLoss >= 0 ? 'diff-positive' : 'diff-negative';
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

    // MAE/MFE - Show 0.00% when values exist but are zero, show '-' only when null/undefined
    const mae = pos.max_adverse_excursion;
    const mfe = pos.max_favorable_excursion;
    const maeText = (mae !== null && mae !== undefined) ? `${mae.toFixed(2)}%` : '-';
    const mfeText = (mfe !== null && mfe !== undefined) ? `${mfe.toFixed(2)}%` : '-';

    const strategyText = formatStrategyName(pos.strategy || 'TRACKING');

    row.innerHTML = `
        <td><strong>${pos.stock_symbol}</strong></td>
        <td style="font-size: 0.85em;">${strategyText}</td>
        <td style="font-size: 0.85em;">${entryTime}</td>
        <td class="text-right">${formatNumber(pos.entry_price)}</td>
        <td class="text-right">
            <span class="${profitClass}" style="font-weight: 600; font-size: 1.1em;">
                ${profitSign}${profitLoss.toFixed(2)}%
            </span>
        </td>
        <td class="text-right" style="font-size: 0.9em;">${holdingText}</td>
        <td class="text-right" style="font-size: 0.85em;">
            <span class="diff-negative">${maeText}</span> /
            <span class="diff-positive">${mfeText}</span>
        </td>
        <td>
            <span class="badge" style="background: var(--accent-blue); color: white;">
                ${pos.outcome_status}
            </span>
        </td>
    `;

    row.addEventListener('mouseenter', () => {
        row.style.backgroundColor = 'rgba(59, 130, 246, 0.05)';
    });
    row.addEventListener('mouseleave', () => {
        row.style.backgroundColor = '';
    });

    return row;
}

/**
 * Render accumulation/distribution summary table
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

        const netValueClass = item.net_value >= 0 ? 'diff-positive' : 'diff-negative';
        const netValueSign = item.net_value >= 0 ? '+' : '';

        row.innerHTML = `
            <td data-label="Saham" class="col-symbol">${item.stock_symbol}</td>
            <td data-label="BUY %" class="text-right">
                <span class="diff-positive" style="font-weight: 600;">${item.buy_percentage.toFixed(1)}%</span>
            </td>
            <td data-label="SELL %" class="text-right">
                <span class="diff-negative" style="font-weight: 600;">${item.sell_percentage.toFixed(1)}%</span>
            </td>
            <td data-label="Net Value" class="text-right">
                <span class="${netValueClass}" style="font-weight: 600;">${netValueSign}${formatCurrency(Math.abs(item.net_value))}</span>
            </td>
            <td data-label="Alerts" class="text-right">${item.total_count}</td>
            <td data-label="Total Value" class="text-right value-highlight">${formatCurrency(item.total_value)}</td>
        `;

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
    const buyVol = stats.buy_volume_lots || 0;
    const sellVol = stats.sell_volume_lots || 0;
    const largestVal = stats.largest_trade_value || 0;
    const winRate = stats.win_rate || 0;
    const avgProfit = stats.avg_profit_pct || 0;

    const totalAlertsEl = document.getElementById('total-alerts');
    const totalVolumeEl = document.getElementById('total-volume');
    const largestValueEl = document.getElementById('largest-value');
    const winRateEl = document.getElementById('global-win-rate');
    const avgProfitEl = document.getElementById('global-avg-profit');

    if (totalAlertsEl) totalAlertsEl.innerText = formatNumber(totalTrades);
    if (totalVolumeEl) totalVolumeEl.innerText = formatNumber(buyVol + sellVol) + " Lot";
    if (largestValueEl) largestValueEl.innerText = formatCurrency(largestVal);

    if (winRateEl) {
        winRateEl.innerText = formatPercent(winRate);
        // Color coding for Win Rate
        if (winRate >= 50) winRateEl.style.color = 'var(--diff-positive)';
        else if (winRate > 0) winRateEl.style.color = 'var(--accent-gold)';
        else winRateEl.style.color = ''; // Default styling
    }

    if (avgProfitEl) {
        avgProfitEl.innerText = (avgProfit > 0 ? '+' : '') + formatPercent(avgProfit);
        // Color coding for Avg Profit
        if (avgProfit > 0) avgProfitEl.className = 'value diff-positive';
        else if (avgProfit < 0) avgProfitEl.className = 'value diff-negative';
        else avgProfitEl.className = 'value';
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
            <div class="placeholder">
                <span class="placeholder-icon">🔗</span>
                <p>Tidak ada data korelasi ditemukan</p>
            </div>`;
        return;
    }

    correlations.forEach(corr => {
        const card = document.createElement('div');
        card.className = 'correlation-card';

        const coefficient = corr.correlation_coefficient || 0;
        let colorClass = 'neutral';
        let strengthText = 'Netral';

        if (coefficient > 0.7) {
            colorClass = 'positive-strong';
            strengthText = 'Sangat Kuat (Positif)';
        } else if (coefficient > 0.4) {
            colorClass = 'positive';
            strengthText = 'Kuat (Positif)';
        } else if (coefficient < -0.7) {
            colorClass = 'negative-strong';
            strengthText = 'Sangat Kuat (Negatif)';
        } else if (coefficient < -0.4) {
            colorClass = 'negative';
            strengthText = 'Kuat (Negatif)';
        }

        // Format Period
        const period = corr.period === '1hour' ? '1 Jam' : corr.period;

        card.innerHTML = `
            <div class="corr-header">
                <div class="pair">
                    <span class="symbol">${corr.stock_a}</span>
                    <span class="separator">↔</span>
                    <span class="symbol">${corr.stock_b}</span>
                </div>
                <div class="corr-value ${colorClass}">
                    ${coefficient.toFixed(2)}
                </div>
            </div>
            <div class="corr-body">
                <div class="strength-bar">
                    <div class="bar-fill ${colorClass}" style="width: ${Math.abs(coefficient) * 100}%"></div>
                </div>
                <div class="corr-meta">
                    <span class="strength-label">${strengthText}</span>
                    <span class="period-label">Period: ${period}</span>
                </div>
            </div>
        `;

        container.appendChild(card);
    });
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

    // P&L calculation
    const profitLoss = record.profit_loss_pct || 0;
    const profitClass = profitLoss > 0 ? 'diff-positive' : profitLoss < 0 ? 'diff-negative' : '';
    const profitSign = profitLoss > 0 ? '+' : '';

    // Entry time
    const entryTime = record.entry_time ? new Date(record.entry_time).toLocaleString('id-ID', {
        day: '2-digit',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit'
    }) : '-';

    // Exit time
    const exitTime = record.exit_time ? new Date(record.exit_time).toLocaleString('id-ID', {
        day: '2-digit',
        month: 'short',
        hour: '2-digit',
        minute: '2-digit'
    }) : '-';

    // Exit price
    const exitPrice = record.exit_price ? formatNumber(record.exit_price) : '-';

    // MAE/MFE
    const mae = record.max_adverse_excursion;
    const mfe = record.max_favorable_excursion;
    const maeText = (mae !== null && mae !== undefined) ? `${mae.toFixed(2)}%` : '-';
    const mfeText = (mfe !== null && mfe !== undefined) ? `${mfe.toFixed(2)}%` : '-';

    // Status badge
    let statusBadge = '';
    const status = record.outcome_status || 'UNKNOWN';
    if (status === 'WIN') {
        statusBadge = '<span class="badge" style="background: var(--diff-positive); color: white;">WIN</span>';
    } else if (status === 'LOSS') {
        statusBadge = '<span class="badge" style="background: var(--diff-negative); color: white;">LOSS</span>';
    } else if (status === 'BREAKEVEN') {
        statusBadge = '<span class="badge" style="background: var(--text-secondary); color: white;">BREAKEVEN</span>';
    } else if (status === 'OPEN') {
        statusBadge = '<span class="badge" style="background: var(--accent-blue); color: white;">OPEN</span>';
    } else if (status === 'SKIPPED') {
        statusBadge = '<span class="badge" style="background: var(--accent-gold); color: white;">SKIPPED</span>';
    } else {
        statusBadge = `<span class="badge" style="background: #666; color: white;">${status}</span>`;
    }

    // Exit reason with enhanced formatting
    const exitReason = record.exit_reason || '-';
    let exitReasonText = exitReason;
    
    // Map standard exit reasons
    if (exitReason === 'TAKE_PROFIT' || exitReason.includes('TAKE_PROFIT')) {
        exitReasonText = '🎯 Take Profit';
    } else if (exitReason === 'STOP_LOSS') {
        exitReasonText = '🛑 Stop Loss';
    } else if (exitReason === 'TIME_BASED') {
        exitReasonText = '⏰ Time Exit';
    } else if (exitReason === 'REVERSE_SIGNAL') {
        exitReasonText = '🔄 Reverse Signal';
    } else if (exitReason === 'MARKET_CLOSE') {
        exitReasonText = '🔚 Market Close';
    }
    // Handle skipped reasons with better formatting
    else if (exitReason.includes('cooldown')) {
        exitReasonText = '⏸️ Cooldown';
    } else if (exitReason.includes('too soon')) {
        exitReasonText = '⏱️ Too Soon';
    } else if (exitReason.includes('already has')) {
        exitReasonText = '🔒 Position Exists';
    } else if (exitReason.includes('Only BUY')) {
        exitReasonText = '❌ SELL Not Supported';
    } else if (exitReason.includes('Signal too soon')) {
        exitReasonText = '⏱️ Signal Too Soon';
    }
    // For any other text, truncate if too long
    else if (exitReasonText.length > 30) {
        exitReasonText = `<span title="${exitReason}">${exitReason.substring(0, 27)}...</span>`;
    }

    // Strategy
    const strategyText = formatStrategyName(record.strategy || 'N/A');

    // Holding duration
    const holdingDuration = record.holding_duration_display || '-';

    row.innerHTML = `
        <td><strong>${record.stock_symbol}</strong></td>
        <td style="font-size: 0.85em;">${strategyText}</td>
        <td style="font-size: 0.85em;">${entryTime}</td>
        <td class="text-right">${formatNumber(record.entry_price)}</td>
        <td style="font-size: 0.85em;">${exitTime}</td>
        <td class="text-right">${exitPrice}</td>
        <td class="text-right">
            <span class="${profitClass}" style="font-weight: 600; font-size: 1.1em;">
                ${profitSign}${profitLoss.toFixed(2)}%
            </span>
        </td>
        <td class="text-right" style="font-size: 0.9em;">${holdingDuration}</td>
        <td class="text-right" style="font-size: 0.85em;">
            <span class="diff-negative">${maeText}</span> /
            <span class="diff-positive">${mfeText}</span>
        </td>
        <td>${statusBadge}</td>
        <td style="font-size: 0.85em;">${exitReasonText}</td>
    `;

    // Hover effect based on P&L
    row.addEventListener('mouseenter', () => {
        if (profitLoss > 0) {
            row.style.backgroundColor = 'rgba(14, 203, 129, 0.05)';
        } else if (profitLoss < 0) {
            row.style.backgroundColor = 'rgba(246, 70, 93, 0.05)';
        } else {
            row.style.backgroundColor = 'rgba(59, 130, 246, 0.05)';
        }
    });
    row.addEventListener('mouseleave', () => {
        row.style.backgroundColor = '';
    });

    return row;
}
