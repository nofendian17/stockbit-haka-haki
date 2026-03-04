// Sparkline generator for tiny inline charts
export function createSparkline(canvasId, data, isPositive = true) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Canvas styling
    const width = canvas.width;
    const height = canvas.height;
    ctx.clearRect(0, 0, width, height);

    if (!data || data.length < 2) return;

    const min = Math.min(...data);
    const max = Math.max(...data);
    const range = max - min || 1;

    // Colors
    const strokeColor = isPositive ? '#0ecb81' : '#f6465d'; // accentSuccess / accentDanger
    const fillColor = isPositive ? 'rgba(14, 203, 129, 0.2)' : 'rgba(246, 70, 93, 0.2)';

    ctx.beginPath();
    ctx.moveTo(0, height);

    // Draw the line
    data.forEach((val, i) => {
        const x = (i / (data.length - 1)) * width;
        const y = height - ((val - min) / range) * (height * 0.8) - (height * 0.1); // Add 10% padding top/bottom

        if (i === 0) {
            ctx.moveTo(x, y);
            ctx.lineTo(x, y); // Start point for fill
        } else {
            ctx.lineTo(x, y);
        }
    });

    ctx.strokeStyle = strokeColor;
    ctx.lineWidth = 1.5;
    ctx.lineCap = 'round';
    ctx.lineJoin = 'round';
    ctx.stroke();

    // Fill area under the curve
    ctx.lineTo(width, height);
    ctx.lineTo(0, height);
    ctx.closePath();
    ctx.fillStyle = fillColor;
    ctx.fill();
}
