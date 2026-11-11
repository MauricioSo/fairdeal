// Pareto Frontier Canvas 2D Renderer (60 FPS)

export class ParetoVisualizer {
  constructor(canvasEl, captionEl) {
    this.canvas = canvasEl;
    this.caption = captionEl;
    this.ctx = canvasEl.getContext('2d');
    this.animFrame = null;
    this.frontiers = [];
    this.nashPoint = [0.5, 0.5];
    this.selectedFrontier = null;
    this.animationStart = 0;
    this.duration = 1600; // 1.6s easeOutCubic
  }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.getBoundingClientRect();
    this.width = rect.width || 480;
    this.height = rect.height || 220;

    this.canvas.width = this.width * dpr;
    this.canvas.height = this.height * dpr;
    this.ctx.resetTransform();
    this.ctx.scale(dpr, dpr);
  }

  setTrace(traceData, latencyUs) {
    this.resize();
    if (!traceData || !traceData.frontiers || traceData.frontiers.length === 0) {
      return;
    }

    this.frontiers = traceData.frontiers;
    this.selectedFrontier = this.frontiers.find(f => f.selected) || this.frontiers[0];
    this.nashPoint = traceData.nash_point || this.selectedFrontier.nash_point || [0.5, 0.5];

    // Find bounding box across all points for consistent normalization
    let maxUb = 0.001;
    let maxUs = 0.001;
    for (const f of this.frontiers) {
      for (const pt of f.points) {
        if (pt[0] > maxUb) maxUb = pt[0];
        if (pt[1] > maxUs) maxUs = pt[1];
      }
    }

    // Normalize points to [0.05, 0.95] for aesthetic padding
    this.normalizedFrontiers = this.frontiers.map(f => {
      const normPts = f.points.map(pt => [
        pt[0] / maxUb,
        pt[1] / maxUs
      ]);
      return {
        label: f.label,
        selected: f.selected,
        points: normPts
      };
    });

    this.normNash = [
      this.nashPoint[0] / maxUb,
      this.nashPoint[1] / maxUs
    ];

    if (this.caption && latencyUs) {
      const ms = (latencyUs / 1000).toFixed(2);
      this.caption.textContent = `Solved in ${ms} ms · animated for clarity`;
    }

    // Check prefers-reduced-motion
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reducedMotion) {
      this.drawFrame(1.0);
    } else {
      this.startAnimation();
    }
  }

  startAnimation() {
    if (this.animFrame) cancelAnimationFrame(this.animFrame);
    this.animationStart = performance.now();

    const loop = (now) => {
      const elapsed = now - this.animationStart;
      const progress = Math.min(1.0, elapsed / this.duration);
      // easeOutCubic: 1 - pow(1 - progress, 3)
      const ease = 1 - Math.pow(1 - progress, 3);

      this.drawFrame(ease);

      if (progress < 1.0) {
        this.animFrame = requestAnimationFrame(loop);
      }
    };
    this.animFrame = requestAnimationFrame(loop);
  }

  drawFrame(t) {
    const ctx = this.ctx;
    const w = this.width;
    const h = this.height;

    ctx.clearRect(0, 0, w, h);

    const padLeft = 46;
    const padBottom = 34;
    const padTop = 18;
    const padRight = 24;

    const plotW = w - padLeft - padRight;
    const plotH = h - padTop - padBottom;

    // Coordinate transforms: X is Merchant Us, Y is Buyer Ub (flipped)
    const toCanvasX = (us) => padLeft + us * plotW;
    const toCanvasY = (ub) => padTop + (1 - ub) * plotH;

    // 1. Grid & Axes (Clean Swiss Light Palette)
    ctx.strokeStyle = '#e2e8f0';
    ctx.lineWidth = 1;
    ctx.beginPath();
    // Horizontal grid
    for (let i = 0.25; i <= 1.0; i += 0.25) {
      const y = toCanvasY(i);
      ctx.moveTo(padLeft, y);
      ctx.lineTo(w - padRight, y);
    }
    // Vertical grid
    for (let i = 0.25; i <= 1.0; i += 0.25) {
      const x = toCanvasX(i);
      ctx.moveTo(x, padTop);
      ctx.lineTo(x, h - padBottom);
    }
    ctx.stroke();

    // Axes lines
    ctx.strokeStyle = '#94a3b8';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(padLeft, padTop);
    ctx.lineTo(padLeft, h - padBottom);
    ctx.lineTo(w - padRight, h - padBottom);
    ctx.stroke();

    // Labels
    ctx.fillStyle = '#64748b';
    ctx.font = '10px -apple-system, sans-serif';
    ctx.fillText('0', padLeft - 12, h - padBottom + 12);
    ctx.fillText('No deal (BATNA)', padLeft + 4, h - padBottom + 14);

    // Axis titles
    ctx.save();
    ctx.translate(14, h / 2 + 10);
    ctx.rotate(-Math.PI / 2);
    ctx.fillStyle = '#0070ba';
    ctx.font = 'bold 11px -apple-system, sans-serif';
    ctx.fillText('Buyer Surplus (Ub)', 0, 0);
    ctx.restore();

    ctx.fillStyle = '#4f46e5';
    ctx.font = 'bold 11px -apple-system, sans-serif';
    ctx.fillText('Merchant Surplus (Us) →', w / 2 - 40, h - 8);

    if (!this.normalizedFrontiers) return;

    // 2. Draw inactive frontiers in subtle slate
    for (const f of this.normalizedFrontiers) {
      if (f.selected) continue;
      this.drawCurve(f.points, toCanvasX, toCanvasY, '#cbd5e1', 1.5, false);
    }

    // 3. Draw active selected frontier
    const activeF = this.normalizedFrontiers.find(f => f.selected) || this.normalizedFrontiers[0];
    if (activeF && activeF.points.length > 0) {
      this.drawCurve(activeF.points, toCanvasX, toCanvasY, '#0070ba', 2.75, true);

      // Label on the selected curve
      const midPt = activeF.points[Math.floor(activeF.points.length / 2)];
      ctx.fillStyle = '#0070ba';
      ctx.font = 'bold 10px -apple-system, sans-serif';
      ctx.fillText(activeF.label, toCanvasX(midPt[1]) - 30, toCanvasY(midPt[0]) - 8);

      // 4. Converging animation points
      const pts = activeF.points;
      const bIdeal = pts[0]; // Highest buyer utility
      const sIdeal = pts[pts.length - 1]; // Highest seller utility

      const nashX = toCanvasX(this.normNash[1]);
      const nashY = toCanvasY(this.normNash[0]);

      // Buyer dot sliding down curve
      const curBX = toCanvasX(bIdeal[1]) + (nashX - toCanvasX(bIdeal[1])) * t;
      const curBY = toCanvasY(bIdeal[0]) + (nashY - toCanvasY(bIdeal[0])) * t;

      // Merchant dot sliding up curve
      const curSX = toCanvasX(sIdeal[1]) + (nashX - toCanvasX(sIdeal[1])) * t;
      const curSY = toCanvasY(sIdeal[0]) + (nashY - toCanvasY(sIdeal[0])) * t;

      if (t < 0.98) {
        ctx.fillStyle = '#0284c7';
        ctx.beginPath();
        ctx.arc(curBX, curBY, 4.5, 0, Math.PI * 2);
        ctx.fill();

        ctx.fillStyle = '#6366f1';
        ctx.beginPath();
        ctx.arc(curSX, curSY, 4.5, 0, Math.PI * 2);
        ctx.fill();
      } else {
        // Collided at Nash Equilibrium Point
        ctx.fillStyle = '#059669';
        ctx.shadowColor = 'rgba(5, 150, 105, 0.35)';
        ctx.shadowBlur = 10;
        ctx.beginPath();
        ctx.arc(nashX, nashY, 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.shadowBlur = 0;

        // Nash label
        ctx.fillStyle = '#059669';
        ctx.font = 'bold 11px -apple-system, sans-serif';
        ctx.fillText('★ Nash Point', nashX + 8, nashY - 6);
      }
    }
  }

  drawCurve(pts, toX, toY, color, width, hasGradient) {
    if (pts.length < 2) return;
    const ctx = this.ctx;

    ctx.strokeStyle = color;
    ctx.lineWidth = width;
    ctx.beginPath();
    ctx.moveTo(toX(pts[0][1]), toY(pts[0][0]));

    for (let i = 1; i < pts.length; i++) {
      ctx.lineTo(toX(pts[i][1]), toY(pts[i][0]));
    }
    ctx.stroke();
  }
}
