// Audio Alert Utility
export const AlertSounds = {
    // A simple tiny "pop" sound for general notifications
    BUY: 'data:audio/wav;base64,UklGRmYBAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YUMBAACAgoSGiIqLjY6PkJGSlJWXmJmam5ydnp+goaKjpKanqKmqq6ytrq+wsbKztLW2t7i5uru8vb6/wMHCw8TFxsfIycrLzM3Oz9DR0tPU1dbX2Nna29zd3t/g4eLj5OXm5+jp6uvs7e7v8PHy8/T19vf4+fr7/P3+/wABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4fICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj9AQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVpbXF1eX2BhYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5ent8fX5/gA==',
    // A slightly different pitch for SELL
    SELL: 'data:audio/wav;base64,UklGRmYBAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YUMBAACAg4aJjI+Rk5WXmZudn6GjpKanqKmqq6ytrq+wsbKztLW2t7i5uru8vb6/wMHCw8TFxsfIycrLzM3Oz9DR0tPU1dbX2Nna29zd3t/g4eLj5OXm5+jp6uvs7e7v8PHy8/T19vf4+fr7/P3+/wABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4fICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj9AQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVpbXF1eX2BhYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5ent8fX5/gIGCg4SFhoeIiYqLjI2Oj5CRkpOUlZaXmJmQ==',
    // A more prominent sound for WHALES
    WHALE: 'data:audio/wav;base64,UklGRuQBAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YLwBAACAg4eJjY+SlJaanJ+ho6Wnqausrq+xtba4ury/wcTGyMrMz9LU1tnc3+Hk5+rr7vDx8/X4+fv+/wIDBQcICwwPDxESFBUZGhweISQnKSwvMTQ2ODs9P0FCQ0ZISkxOUFFTVFVXWFlbXV9hY2VmZ2lqa2xub3Bxc3R1dnd4eXp7fH1+f4CBgoOEhYaHiImKi4yNjo+QkZKTlJWWl5iZmpucnZ6foKGio6SlpqeoqaqrrK2ur7CxsrO0tba3uLm6u7y9vr/AwcLDxMXGx8jJysvMzc7P0NHS09TV1tfY2drb3N3e3+Dh4uPk5ebn6Onq6+zt7u/w8fLz9PX29/j5+vv8/f7/AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0BBQkNERUZHSElKS0xNTk9QUVJTVFVWV1hZWlvcXV5fYGFiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eHl6e3x9fn+AA=='
};

// State for notification permission
let isNotificationEnabled = false;

export function initNotifications() {
    if ('Notification' in window) {
        if (Notification.permission === 'granted') {
            isNotificationEnabled = true;
        } else if (Notification.permission !== 'denied') {
            // We can ask when user clicks a button, not immediately
        }
    }
}

export function requestNotificationPermission() {
    if (!('Notification' in window)) return;

    Notification.requestPermission().then(permission => {
        isNotificationEnabled = (permission === 'granted');
    });
}

export function playSound(type = 'WHALE') {
    try {
        const audioUrl = AlertSounds[type] || AlertSounds.WHALE;
        const audio = new Audio(audioUrl);
        audio.volume = 0.5;
        // The play() might fail if user hasn't interacted with the document yet
        audio.play().catch(e => console.log('Audio playback prevented by browser:', e));
    } catch(e) {}
}

export function showDesktopNotification(title, body, icon = '🐋') {
    if (!isNotificationEnabled || !('Notification' in window)) return;

    try {
        new Notification(title, {
            body: body,
            icon: '/favicon.ico' // Or a generated emoji icon
        });
    } catch(e) {
        console.error('Notification error', e);
    }
}

// Global toggle state handled in UI
export const AppSettings = {
    soundEnabled: true,
    desktopNotifications: false
};
