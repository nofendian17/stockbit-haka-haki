function renderSignalCardRefactored(signal) {
    const container = document.getElementById('signals-container');
    
    const signalId = `${signal.stock_symbol}-${signal.strategy}-${signal.timestamp}`;
    if (renderedSignalIds.has(signalId)) return;

    // Determine colors and labels
    let actionColor = '#707a8a';
    let actionLabel = signal.decision;
    let actionIcon = '⚪';
    let cardClass = '';
    let badgeClass = '';

    if (signal.decision === 'BUY') {
        actionColor = '#0ECB81';
        actionIcon = '🟢';
        cardClass = 'buy-signal';
        badgeClass = 'buy-badge';
    } else if (signal.decision === 'SELL') {
        actionColor = '#F6465D';
        actionIcon = '🔴';
        cardClass = 'sell-signal';
        badgeClass = 'sell-badge';
    } else if (signal.decision === 'WAIT') {
        actionColor = '#FFD700';
        actionIcon = '🟡';
        cardClass = 'wait-signal';
        badgeClass = 'wait-badge';
    }

    // Strategy icon mapping
    const strategyIcons = {
        'VOLUME_BREAKOUT': '🚀',
        'MEAN_REVERSION': '↩️',
        'FAKEOUT_FILTER': '🛡️'
    };
    const strategyIcon = strategyIcons[signal.strategy] || '📊';

    // Format Data
    const priceFormatted = new Intl.NumberFormat('id-ID').format(signal.price);
    const changeFormatted = Math.abs(signal.change).toFixed(2);
    const changeSign = signal.change >= 0 ? '+' : '-';
    const changeColor = signal.change >= 0 ? '#0ECB81' : '#F6465D';
    const changeIcon = signal.change >= 0 ? '📈' : '📉';
    
    // Confidence Calc
    const confidencePercent = Math.round(signal.confidence * 100);
    
    // Confidence level text
    let confidenceLevel = 'Low';
    let confidenceLevelClass = 'conf-low';
    if (confidencePercent >= 80) {
        confidenceLevel = 'Very High';
        confidenceLevelClass = 'conf-very-high';
    } else if (confidencePercent >= 60) {
        confidenceLevel = 'High';
        confidenceLevelClass = 'conf-high';
    } else if (confidencePercent >= 40) {
        confidenceLevel = 'Medium';
        confidenceLevelClass = 'conf-medium';
    }

    // SVG Circle params for confidence ring
    const radius = 20;
    const circumference = 2 * Math.PI * radius;
    const offset = circumference - (signal.confidence * circumference);

    // Simplified Reason (Extract key info)
    let simpleReason = signal.reason;
    // Highlight key terms with better formatting
    simpleReason = simpleReason.replace(/(Z=[-]?\d+\.?\d*)/g, '<strong class="reason-highlight">$1</strong>');
    simpleReason = simpleReason.replace(/(\d+\.?\d*%)/g, '<strong class="reason-highlight">$1</strong>');
    simpleReason = simpleReason.replace(/(volume|price|breakout|support|resistance)/gi, '<em class="reason-keyword">$1</em>');

    // Time Ago
    const timeAgo = getTimeAgo(new Date(signal.timestamp));
    const timestamp = new Date(signal.timestamp).toLocaleString('id-ID', { 
        hour: '2-digit', 
        minute: '2-digit',
        day: '2-digit',
        month: 'short'
    });

    const card = document.createElement('div');
    card.className = `signal-card ${cardClass}`;
    
    card.innerHTML = `
        <div class="card-top-stripe"></div>
        
        <div class="card-header-enhanced">
            <div class="symbol-section">
                <div class="symbol-main">
                    <span class="symbol-icon">📊</span>
                    <h3 class="card-symbol">${signal.stock_symbol}</h3>
                </div>
                <div class="strategy-badge">
                    <span class="strategy-icon">${strategyIcon}</span>
                    <span class="strategy-name">${formatStrategyName(signal.strategy)}</span>
                </div>
            </div>
            <div class="action-badge-enhanced ${badgeClass}">
                <span class="action-icon">${actionIcon}</span>
                <span class="action-text">${actionLabel}</span>
            </div>
        </div>
        
        <div class="card-metrics">
            <div class="metric-item price-metric">
                <div class="metric-label">
                    <span class="metric-icon">💵</span>
                    <span>Price</span>
                </div>
                <div class="metric-value price-value">Rp ${priceFormatted}</div>
            </div>
            
            <div class="metric-divider"></div>
            
            <div class="metric-item change-metric">
                <div class="metric-label">
                    <span class="metric-icon">${changeIcon}</span>
                    <span>Change</span>
                </div>
                <div class="metric-value change-value" style="color: ${changeColor}">
                    ${changeSign}${changeFormatted}%
                </div>
            </div>
            
            <div class="metric-divider"></div>
            
            <div class="metric-item confidence-metric">
                <div class="metric-label">
                    <span class="metric-icon">🎯</span>
                    <span>Confidence</span>
                </div>
                <div class="confidence-display">
                    <svg width="50" height="50" class="confidence-ring">
                        <circle class="conf-bg" stroke-width="4" fill="transparent" r="${radius}" cx="25" cy="25"></circle>
                        <circle class="conf-fg" stroke="${actionColor}" stroke-width="4" fill="transparent" 
                                r="${radius}" cx="25" cy="25" 
                                style="stroke-dasharray: ${circumference}; stroke-dashoffset: ${offset}; transform: rotate(-90deg); transform-origin: center;">
                        </circle>
                        <text x="50%" y="50%" class="conf-percent" fill="${actionColor}" text-anchor="middle" dominant-baseline="middle">
                            ${confidencePercent}%
                        </text>
                    </svg>
                    <div class="confidence-label ${confidenceLevelClass}">${confidenceLevel}</div>
                </div>
            </div>
        </div>

        <div class="card-reason">
            <div class="reason-header">
                <span class="reason-icon">💡</span>
                <span class="reason-title">Analysis</span>
            </div>
            <div class="reason-content">
                ${simpleReason}
            </div>
        </div>

        <div class="card-footer-enhanced">
            <div class="time-info">
                <span class="time-icon">🕐</span>
                <span class="time-relative">${timeAgo}</span>
                <span class="time-separator">•</span>
                <span class="time-absolute">${timestamp}</span>
            </div>
        </div>
    `;

    if (container.firstChild) {
        container.insertBefore(card, container.firstChild);
    } else {
        container.appendChild(card);
    }

    renderedSignalIds.add(signalId);

    if (container.children.length > MAX_VISIBLE_SIGNALS) {
        if (container.lastChild) container.removeChild(container.lastChild);
    }
}

function formatStrategyName(strategy) {
    return strategy.replace(/_/g, ' ').toLowerCase().replace(/\b\w/g, l => l.toUpperCase());
}

function getTimeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    if (seconds < 60) return 'Just now';
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
}
