/**
 * Mobile Table Helper
 * Adds data-label attributes to table cells for card-based mobile layout
 */

function initMobileTables() {
    // Only process on mobile viewports
    if (window.innerWidth <= 768) {
        document.querySelectorAll('.data-table').forEach(table => {
            // Get headers text
            const headers = Array.from(table.querySelectorAll('thead th')).map(th =>
                th.textContent.trim()
            );

            // Add data-label to each cell
            table.querySelectorAll('tbody tr').forEach(row => {
                row.querySelectorAll('td').forEach((cell, index) => {
                    if (headers[index]) {
                        cell.setAttribute('data-label', headers[index]);
                    }
                });
            });
        });
    }
}

// Initialize on load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initMobileTables);
} else {
    initMobileTables();
}

// Re-initialize on window resize (debounced)
let resizeTimeout;
window.addEventListener('resize', () => {
    clearTimeout(resizeTimeout);
    resizeTimeout = setTimeout(initMobileTables, 250);
});

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { initMobileTables };
}

// Also make available globally for dynamic content
window.initMobileTables = initMobileTables;
