/**
 * Utility Functions
 * Shared helper functions for formatting and data manipulation
 */

/**
 * Format number as Indonesian Rupiah currency
 * @param {number} val - The value to format
 * @returns {string} Formatted currency string
 */
export function formatCurrency(val) {
    if (!val || isNaN(val)) return 'Rp 0';

    if (val >= 1_000_000_000_000) {
        return `Rp ${(val / 1_000_000_000_000).toFixed(2)} T`;
    } else if (val >= 1_000_000_000) {
        return `Rp ${(val / 1_000_000_000).toFixed(2)} M`;
    } else if (val >= 1_000_000) {
        return `Rp ${(val / 1_000_000).toFixed(2)} Jt`;
    }
    return `Rp ${new Intl.NumberFormat('id-ID').format(val)}`;
}

/**
 * Format large numbers with K/M/B suffixes
 * @param {number} val - The value to format
 * @returns {string} Formatted number string
 */
export function formatNumber(val) {
    if (!val || isNaN(val)) return '0';

    if (val >= 1_000_000_000) {
        return `${(val / 1_000_000_000).toFixed(1)}B`;
    } else if (val >= 1_000_000) {
        return `${(val / 1_000_000).toFixed(1)}M`;
    } else if (val >= 1_000) {
        return `${(val / 1_000).toFixed(1)}K`;
    }
    return new Intl.NumberFormat('id-ID').format(val);
}

/**
 * Format ISO timestamp to Indonesian locale
 * @param {string} isoString - ISO timestamp string
 * @returns {string} Formatted time string
 */
export function formatTime(isoString) {
    if (!isoString) return '-';

    try {
        const date = new Date(isoString);
        if (isNaN(date.getTime())) return '-';

        return date.toLocaleString('id-ID', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit'
        });
    } catch (err) {
        console.error('Error formatting time:', err);
        return '-';
    }
}

/**
 * Format percentage value
 * @param {number} val - The percentage value
 * @returns {string} Formatted percentage string
 */
export function formatPercent(val) {
    if (val === null || val === undefined || isNaN(val)) return '0%';
    return `${val.toFixed(2)}%`;
}

/**
 * Get relative time string (e.g., "5m ago", "2h ago")
 * @param {Date} date - The date to compare
 * @returns {string} Relative time string
 */
export function getTimeAgo(date) {
    if (!date || !(date instanceof Date) || isNaN(date.getTime())) {
        return 'Unknown';
    }

    const seconds = Math.floor((new Date() - date) / 1000);

    if (seconds < 10) return 'Just now';
    if (seconds < 60) return `${seconds}s ago`;

    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;

    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;

    const days = Math.floor(hours / 24);
    if (days === 1) return 'Yesterday';
    if (days < 7) return `${days}d ago`;

    return date.toLocaleDateString('id-ID', { month: 'short', day: 'numeric' });
}

/**
 * Safely parse a date from various timestamp formats
 * @param {string|number|Date} timestamp - The timestamp to parse
 * @returns {Date|null} Parsed date or null if invalid
 */
export function parseTimestamp(timestamp) {
    if (!timestamp) return null;

    try {
        const date = new Date(timestamp);
        if (!isNaN(date.getTime()) && date.getTime() > 0) {
            return date;
        }
    } catch (err) {
        console.error('Error parsing timestamp:', timestamp, err);
    }

    return null;
}

/**
 * Safely get DOM element with error logging
 * @param {string} id - Element ID
 * @param {string} context - Context for error message
 * @returns {HTMLElement|null}
 */
export function safeGetElement(id, context = 'App') {
    const element = document.getElementById(id);
    if (!element) {
        console.warn(`[${context}] Element not found: ${id}`);
    }
    return element;
}

/**
 * Debounce function to limit function calls
 * @param {Function} func - Function to debounce
 * @param {number} wait - Wait time in milliseconds
 * @returns {Function} Debounced function
 */
export function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

/**
 * Format strategy name for display
 * @param {string} strategy - Strategy name in UPPER_SNAKE_CASE
 * @returns {string} Formatted strategy name
 */
export function formatStrategyName(strategy) {
    if (!strategy) return '-';
    return strategy
        .replace(/_/g, ' ')
        .toLowerCase()
        .replace(/\b\w/g, l => l.toUpperCase());
}

/**
 * Clamp a number between min and max
 * @param {number} value - Value to clamp
 * @param {number} min - Minimum value
 * @param {number} max - Maximum value
 * @returns {number} Clamped value
 */
export function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
}

/**
 * Get color for market regime
 * @param {string} regime - Market regime code
 * @returns {string} Color hex code or var
 */
export function getRegimeColor(regime) {
    switch (regime) {
        case 'TRENDING_UP': return 'var(--diff-positive)';
        case 'TRENDING_DOWN': return 'var(--diff-negative)';
        case 'RANGING': return 'var(--accent-gold)';
        case 'BREAKOUT': return '#9B59B6'; // Purple
        case 'BREAKDOWN': return '#E67E22'; // Orange
        case 'VOLATILE': return '#E74C3C';
        default: return '#7F8C8D';
    }
}

/**
 * Get display label for market regime
 * @param {string} regime - Market regime code
 * @returns {string} Display label
 */
export function getRegimeLabel(regime) {
    if (!regime) return 'UNKNOWN';
    return regime.replace(/_/g, ' ');
}
