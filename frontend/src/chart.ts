// Petite courbe en direct sur canvas, sans dépendance.
export class LiveChart {
  private xs: number[] = [];
  private ys: number[] = [];
  private canvas: HTMLCanvasElement;

  constructor(canvas: HTMLCanvasElement, private color: string, private max = 900) {
    this.canvas = canvas;
    new ResizeObserver(() => this.draw()).observe(canvas);
  }

  reset() { this.xs = []; this.ys = []; this.draw(); }

  push(x: number, y: number) {
    this.xs.push(x);
    this.ys.push(y);
    if (this.xs.length > this.max) {
      // Réduction : un point sur deux.
      this.xs = this.xs.filter((_, i) => i % 2 === 0);
      this.ys = this.ys.filter((_, i) => i % 2 === 0);
    }
    this.draw();
  }

  draw() {
    const c = this.canvas;
    const dpr = window.devicePixelRatio || 1;
    const w = c.clientWidth, h = c.clientHeight;
    if (!w || !h) return;
    if (c.width !== w * dpr || c.height !== h * dpr) { c.width = w * dpr; c.height = h * dpr; }
    const g = c.getContext('2d')!;
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, w, h);
    const pts = this.ys.map((y, i) => [this.xs[i], y]).filter(p => Number.isFinite(p[1]));
    const ml = 40, mr = 8, mt = 8, mb = 18;
    g.font = '11px system-ui, sans-serif';
    g.fillStyle = '#8a93a6';
    g.strokeStyle = '#e9ecf1';
    g.lineWidth = 1;
    if (pts.length < 2) {
      g.fillText('—', w / 2, h / 2);
      return;
    }
    let ymin = Math.min(...pts.map(p => p[1])), ymax = Math.max(...pts.map(p => p[1]));
    if (ymax - ymin < 1e-9) { ymin -= 1; ymax += 1; }
    const pad = (ymax - ymin) * 0.1; ymin -= pad; ymax += pad;
    if (ymin < 0 && Math.min(...pts.map(p => p[1])) >= 0) ymin = 0;
    const xmin = pts[0][0], xmax = Math.max(pts[pts.length - 1][0], xmin + 1);
    const px = (x: number) => ml + (x - xmin) / (xmax - xmin) * (w - ml - mr);
    const py = (y: number) => mt + (1 - (y - ymin) / (ymax - ymin)) * (h - mt - mb);
    for (let i = 0; i <= 3; i++) {
      const v = ymin + (ymax - ymin) * i / 3;
      const y = py(v);
      g.beginPath(); g.moveTo(ml, y); g.lineTo(w - mr, y); g.stroke();
      g.fillText(v >= 100 ? v.toFixed(0) : v.toFixed(1), 2, y + 4);
    }
    const fmt = (s: number) => { s = Math.round(s); const m = Math.floor(s / 60); return m >= 60 ? `${Math.floor(m / 60)}h${String(m % 60).padStart(2, '0')}` : `${m}:${String(s % 60).padStart(2, '0')}`; };
    g.fillText(fmt(xmin), ml, h - 4);
    const lbl = fmt(xmax); g.fillText(lbl, w - mr - g.measureText(lbl).width, h - 4);
    g.strokeStyle = this.color;
    g.lineWidth = 1.8;
    g.lineJoin = 'round';
    g.beginPath();
    pts.forEach((p, i) => i ? g.lineTo(px(p[0]), py(p[1])) : g.moveTo(px(p[0]), py(p[1])));
    g.stroke();
  }
}
