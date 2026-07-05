(function () {
  'use strict';

  var ACCENT = '#f5a623';
  var POOL = ['#6f8ea8','#7fa389','#a88f6f','#8c7fa8','#a87f86','#6f9aa8','#9aa86f','#a8896f','#7f8ca8','#88a86f','#a86f9a'];

  function svgEl(tag, attrs) {
    var el = document.createElementNS('http://www.w3.org/2000/svg', tag);
    for (var k in attrs) el.setAttribute(k, attrs[k]);
    return el;
  }

  function fmtTime(secs) {
    var d = new Date(secs * 1000);
    var h = d.getHours(), m = d.getMinutes();
    return (h < 10 ? '0' : '') + h + ':' + (m < 10 ? '0' : '') + m;
  }

  function fmtDate(secs) {
    var d = new Date(secs * 1000);
    var day = d.getDate(), mon = d.getMonth() + 1;
    return (day < 10 ? '0' : '') + day + '.' + (mon < 10 ? '0' : '') + mon;
  }

  function fmtLabel(secs, spanDays) {
    return spanDays > 1 ? fmtDate(secs) : fmtTime(secs);
  }

  function fmtScore(v) { return String(Math.round(v)); }

  function niceMax(v) {
    if (v <= 0) return 1000;
    var mag = Math.pow(10, Math.floor(Math.log(v) / Math.LN10));
    var d = v / mag;
    var nice = d <= 1 ? 1 : d <= 2 ? 2 : d <= 5 ? 5 : 10;
    return nice * mag;
  }

  function teamColor(idx, mine) {
    if (idx === 0) return ACCENT;
    if (mine) return '#c0c8d0';
    return POOL[(idx - 1) % POOL.length];
  }

  function valueAt(pts, t) {
    var s = 0;
    for (var i = 0; i < pts.length; i++) {
      if (pts[i].t <= t) s = pts[i].s;
      else break;
    }
    return s;
  }

  function windowPts(pts, tStart, tEnd) {
    var vis = pts.filter(function (p) { return p.s >= 0 && p.t > tStart; });
    vis.unshift({ t: tStart, s: valueAt(pts, tStart) });
    if (vis[vis.length - 1].t < tEnd) {
      vis.push({ t: tEnd, s: vis[vis.length - 1].s });
    }
    var ramp = Math.max(60, (tEnd - tStart) * 0.06);
    var out = [];
    for (var i = 0; i < vis.length; i++) {
      if (i > 0 && vis[i].s !== vis[i - 1].s) {
        var hold = Math.max(vis[i].t - ramp, vis[i - 1].t);
        if (hold > vis[i - 1].t) out.push({ t: hold, s: vis[i - 1].s });
      }
      out.push(vis[i]);
    }
    return out;
  }

  function smoothPath(P) {
    var n = P.length;
    if (!n) return '';
    var d = 'M' + P[0].x.toFixed(1) + ',' + P[0].y.toFixed(1);
    if (n < 3) {
      for (var j = 1; j < n; j++) d += ' L' + P[j].x.toFixed(1) + ',' + P[j].y.toFixed(1);
      return d;
    }
    var dx = [], sl = [], m = [];
    for (var i = 0; i < n - 1; i++) {
      dx.push(P[i + 1].x - P[i].x);
      sl.push(dx[i] ? (P[i + 1].y - P[i].y) / dx[i] : 0);
    }
    m.push(sl[0]);
    for (i = 1; i < n - 1; i++) {
      m.push(sl[i - 1] * sl[i] <= 0 ? 0 : 2 / (1 / sl[i - 1] + 1 / sl[i]));
    }
    m.push(sl[n - 2]);
    for (i = 0; i < n - 1; i++) {
      var h = dx[i] / 3;
      d += ' C' + (P[i].x + h).toFixed(1) + ',' + (P[i].y + m[i] * h).toFixed(1) +
        ' ' + (P[i + 1].x - h).toFixed(1) + ',' + (P[i + 1].y - m[i + 1] * h).toFixed(1) +
        ' ' + P[i + 1].x.toFixed(1) + ',' + P[i + 1].y.toFixed(1);
    }
    return d;
  }

  function showPlaceholder(container, msg) {
    var el = document.createElement('div');
    el.className = 'chart-placeholder';
    el.textContent = msg;
    container.appendChild(el);
  }

  function buildChart(data, container) {
    container.innerHTML = '';

    var cw = container.clientWidth || 1180;
    var VW = Math.max(300, Math.min(1180, cw));
    var narrow = VW < 700;
    var VH = narrow ? 250 : 330;
    var X0 = 8, Y0 = 14, PW = VW - 16, PH = VH - 44;
    var X_LABEL_Y = VH - 8;

    var teams = (data.teams || []).slice();
    if (!teams.length) { showPlaceholder(container, 'Нет данных'); return; }

    teams.sort(function (a, b) {
      var am = 0, bm = 0;
      (a.pts || []).forEach(function (p) { if (p.s > am) am = p.s; });
      (b.pts || []).forEach(function (p) { if (p.s > bm) bm = p.s; });
      return bm - am;
    });

    var visTeams = teams.slice(0, 5);
    for (var vi = 5; vi < teams.length; vi++) {
      if (teams[vi].mine) { visTeams.push(teams[vi]); break; }
    }
    teams = visTeams;

    var allTMin = Infinity, allTMax = -Infinity, sMax = 0;
    var firstScoreT = Infinity;
    teams.forEach(function (team) {
      (team.pts || []).forEach(function (p) {
        if (p.t < allTMin) allTMin = p.t;
        if (p.t > allTMax) allTMax = p.t;
        if (p.s > sMax) sMax = p.s;
        if (p.s > 0 && p.t < firstScoreT) firstScoreT = p.t;
      });
    });

    if (!isFinite(allTMin)) { showPlaceholder(container, 'Недостаточно данных'); return; }
    if (allTMin === allTMax) allTMax = allTMin + 3600;

    var tMin = allTMin;
    if (isFinite(firstScoreT) && (firstScoreT - allTMin) > 1800) {
      var activeSpan = allTMax - firstScoreT;
      var leftPad = Math.max(activeSpan * 0.18, 900);
      tMin = firstScoreT - leftPad;
    }
    var tMax = allTMax;
    var tRange = tMax - tMin;
    var spanDays = (tMax - tMin) / 86400;
    var maxY = niceMax(sMax);

    function px(t) { return Math.round(X0 + ((t - tMin) / tRange) * PW) + 0.5; }
    function py(v) { return Math.round(Y0 + PH - (Math.max(0, Math.min(v, maxY)) / maxY) * PH) + 0.5; }

    var hiddenTeams = {};

    var hovIdx = null;

    var svg = svgEl('svg', {
      viewBox: '0 0 ' + VW + ' ' + VH,
      style: 'width:100%;height:auto;display:block;cursor:crosshair',
      'aria-hidden': 'true'
    });

    var G = 5;
    for (var gi = 0; gi <= G; gi++) {
      var gv = Math.round((gi / G) * maxY);
      var gy = py(gv);
      svg.appendChild(svgEl('line', {
        x1: X0, x2: X0 + PW, y1: gy.toFixed(1), y2: gy.toFixed(1),
        stroke: '#1e232b', 'stroke-width': 1,
        'shape-rendering': 'crispEdges'
      }));
      var labelY = (gv === maxY ? gy + 14 : gy - 5).toFixed(1);
      var gt = svgEl('text', {
        x: 10, y: labelY, fill: '#4a5260', 'font-size': 11,
        'font-family': 'Golos Text, sans-serif', 'text-anchor': 'start'
      });
      gt.textContent = fmtScore(gv);
      svg.appendChild(gt);
    }

    var XN = narrow ? 3 : 5;
    for (var xi = 0; xi <= XN; xi++) {
      var xt = tMin + (xi / XN) * tRange;
      var anchor = xi === 0 ? 'start' : (xi === XN ? 'end' : 'middle');
      var xtxt = svgEl('text', {
        x: px(xt).toFixed(1), y: X_LABEL_Y, fill: '#4a5260', 'font-size': 11,
        'font-family': 'Golos Text, sans-serif', 'text-anchor': anchor
      });
      xtxt.textContent = fmtLabel(xt, spanDays);
      svg.appendChild(xtxt);
    }

    var leader = teams[0];
    var lpts = windowPts(leader.pts || [], tMin, allTMax);
    var lxy = lpts.map(function (p) { return { x: px(p.t), y: py(p.s) }; });
    if (lpts.length >= 2) {
      var bottomY = (Y0 + PH).toFixed(1);
      var areaD = smoothPath(lxy) +
        ' L' + lxy[lxy.length - 1].x.toFixed(1) + ',' + bottomY +
        ' L' + lxy[0].x.toFixed(1) + ',' + bottomY + ' Z';
      var leaderArea = svgEl('path', {
        d: areaD,
        fill: ACCENT,
        'fill-opacity': '0.06',
        'class': 'chart-areaf'
      });
      svg.appendChild(leaderArea);
    }

    var cross = svgEl('line', {
      x1: 0, x2: 0, y1: Y0, y2: Y0 + PH,
      stroke: ACCENT, 'stroke-width': 1, 'stroke-opacity': 0.4,
      'stroke-dasharray': '3 4', display: 'none'
    });
    svg.appendChild(cross);

    var dotsG = svgEl('g', {});
    svg.appendChild(dotsG);

    var nonLeaderEls = [];
    var lineEls = [];

    teams.slice(1).forEach(function (team, i) {
      var vis = windowPts(team.pts || [], tMin, allTMax);
      if (vis.length < 2) return;
      var idx = i + 1;
      var color = teamColor(idx, team.mine);

      var maxScore = 0;
      vis.forEach(function (p) { if (p.s > maxScore) maxScore = p.s; });
      var isZeroTeam = maxScore === 0;

      var lineAttrs = {
        d: smoothPath(vis.map(function (p) { return { x: px(p.t), y: py(p.s) }; })),
        fill: 'none',
        stroke: color,
        'stroke-width': isZeroTeam ? 1 : 2,
        'stroke-opacity': isZeroTeam ? 0.18 : 0.50,
        'stroke-linejoin': 'round',
        'stroke-linecap': 'round',
        pathLength: 1
      };
      if (isZeroTeam) {
        lineAttrs['stroke-dasharray'] = '4 5';
      }
      var ln = svgEl('path', lineAttrs);
      ln.setAttribute('class', 'chart-ln');
      ln.style.animationDelay = (i * 0.05).toFixed(2) + 's';
      ln._teamIdx = idx;
      ln._isZero = isZeroTeam;
      svg.appendChild(ln);
      nonLeaderEls.push(ln);
      lineEls[idx] = ln;
    });

    var leaderEl = null;
    if (lpts.length >= 2) {
      leaderEl = svgEl('path', {
        d: smoothPath(lxy),
        fill: 'none',
        stroke: ACCENT,
        'stroke-width': 3,
        'stroke-opacity': 1,
        'stroke-linejoin': 'round',
        'stroke-linecap': 'round',
        pathLength: 1
      });
      leaderEl.setAttribute('class', 'chart-leaderln');
      leaderEl._teamIdx = 0;
      svg.appendChild(leaderEl);
      lineEls[0] = leaderEl;

      var lp = lpts[lpts.length - 1];
      var lex = px(lp.t).toFixed(1), ley = py(lp.s).toFixed(1);

      var ldot = svgEl('circle', {
        cx: lex, cy: ley, r: 4.5, fill: ACCENT, stroke: '#14161a', 'stroke-width': 2
      });
      ldot.setAttribute('class', 'chart-leaderdot');
      svg.appendChild(ldot);
    }

    var tip = document.createElement('div');
    tip.className = 'chart-tip';
    tip.style.display = 'none';

    var wrap = document.createElement('div');
    wrap.style.position = 'relative';
    wrap.appendChild(svg);
    wrap.appendChild(tip);

    function applyDim() {
      nonLeaderEls.forEach(function (ln) {
        var idx = ln._teamIdx;
        var isHidden = hiddenTeams[idx];
        if (isHidden) {
          ln.setAttribute('stroke-opacity', 0);
        } else if (hovIdx === null) {
          ln.setAttribute('stroke-opacity', ln._isZero ? 0.18 : 0.50);
          ln.setAttribute('stroke-width',   ln._isZero ? 1 : 2);
        } else if (idx === hovIdx) {
          ln.setAttribute('stroke-opacity', 1);
          ln.setAttribute('stroke-width',   3);
        } else {
          ln.setAttribute('stroke-opacity', 0.08);
          ln.setAttribute('stroke-width',   1);
        }
      });

      if (leaderEl) {
        var lhidden = hiddenTeams[0];

        [leaderArea, ldot].forEach(function (el) {
          if (el) el.style.display = lhidden ? 'none' : '';
        });
        if (lhidden) {
          leaderEl.setAttribute('stroke-opacity', 0);
        } else if (hovIdx === null) {
          leaderEl.setAttribute('stroke-opacity', 1);
          leaderEl.setAttribute('stroke-width', 3);
        } else if (hovIdx === 0) {
          leaderEl.setAttribute('stroke-opacity', 1);
          leaderEl.setAttribute('stroke-width', 3);
        } else {
          leaderEl.setAttribute('stroke-opacity', 0.08);
          leaderEl.setAttribute('stroke-width', 1);
        }
      }
    }

    function hideTip() {
      cross.setAttribute('display', 'none');
      tip.style.display = 'none';
      while (dotsG.firstChild) dotsG.removeChild(dotsG.firstChild);
    }

    function scoreAt(rawPts, t) {
      var s = 0;
      for (var i = 0; i < rawPts.length; i++) {
        if (rawPts[i].t <= t) s = rawPts[i].s;
        else break;
      }
      return s;
    }

    function stepAtTime(rawPts, t) {
      for (var i = 1; i < rawPts.length; i++) {
        if (rawPts[i].t <= t && rawPts[i].t > rawPts[i-1].t &&
            rawPts[i].s > rawPts[i-1].s) {

          if (Math.abs(rawPts[i].t - t) < 300) {
            return { delta: rawPts[i].s - rawPts[i-1].s };
          }
        }
      }
      return null;
    }

    svg.addEventListener('mousemove', function (e) {
      var rect = svg.getBoundingClientRect();
      var mx = (e.clientX - rect.left) * (VW / rect.width);
      if (mx < X0 || mx > X0 + PW) { hideTip(); return; }

      var tAt = tMin + ((mx - X0) / PW) * tRange;

      cross.setAttribute('x1', mx.toFixed(1));
      cross.setAttribute('x2', mx.toFixed(1));
      cross.removeAttribute('display');

      var rows = [];
      teams.forEach(function (team, idx) {
        if (hiddenTeams[idx]) return;
        var raw = (team.pts || []);
        var s = scoreAt(raw, tAt);
        rows.push({ name: team.name, score: s, color: teamColor(idx, team.mine), idx: idx, raw: raw });
      });
      rows.sort(function (a, b) { return b.score - a.score; });

      while (dotsG.firstChild) dotsG.removeChild(dotsG.firstChild);
      rows.forEach(function (r) {
        if (r.score <= 0 && hovIdx === null) return;
        if (hovIdx !== null && r.idx !== hovIdx) return;
        var d = svgEl('circle', {
          cx: mx.toFixed(1), cy: py(r.score).toFixed(1),
          r: (hovIdx !== null && r.idx === hovIdx) ? 5 : 3.5,
          fill: r.color, stroke: '#14161a', 'stroke-width': 1.5
        });
        dotsG.appendChild(d);
      });

      while (tip.firstChild) tip.removeChild(tip.firstChild);

      var timeEl = document.createElement('div');
      timeEl.className = 'chart-tip-time';
      timeEl.textContent = fmtLabel(tAt, spanDays);
      tip.appendChild(timeEl);

      var visRows = hovIdx !== null ? rows.filter(function (r) { return r.idx === hovIdx; }) : rows;
      visRows.forEach(function (r) {
        var row = document.createElement('div');
        row.className = 'chart-tip-row';
        var dot = document.createElement('span');
        dot.className = 'chart-tip-dot'; dot.style.background = r.color;
        var name = document.createElement('span');
        name.className = 'chart-tip-name'; name.textContent = r.name;
        var val = document.createElement('span');
        val.className = 'chart-tip-val'; val.textContent = fmtScore(r.score);
        row.appendChild(dot); row.appendChild(name); row.appendChild(val);

        var step = stepAtTime(r.raw, tAt);
        if (step && step.delta > 0) {
          var delta = document.createElement('span');
          delta.style.cssText = 'font-size:11px;color:#f5a623;margin-left:4px;font-variant-numeric:tabular-nums;';
          delta.textContent = '+' + step.delta;
          row.appendChild(delta);
        }
        tip.appendChild(row);
      });

      var leftPct = mx / VW * 100;
      tip.style.display = 'block';
      tip.className = 'chart-tip chart-tipin';
      tip.style.left = leftPct.toFixed(1) + '%';
      tip.style.transform = leftPct > 60 ? 'translateX(calc(-100% - 14px))' : 'translateX(14px)';
    });

    svg.addEventListener('mouseleave', hideTip);

    container.appendChild(wrap);

    var legend = document.createElement('div');
    legend.className = 'chart-legend';

    teams.forEach(function (team, idx) {
      var color = teamColor(idx, team.mine);
      var item = document.createElement('span');
      item.className = 'chart-legend-item';
      item.setAttribute('tabindex', '0');
      item.setAttribute('role', 'button');
      item.setAttribute('aria-pressed', 'false');
      item.title = 'Клик: скрыть/показать';

      var dot = document.createElement('span');
      dot.className = 'chart-legend-dot'; dot.style.background = color;
      var nm = document.createElement('span');
      nm.className = 'chart-legend-name'; nm.textContent = team.name;
      item.appendChild(dot); item.appendChild(nm);

      (function (i, it, d, n) {
        function toggleHidden() {
          hiddenTeams[i] = !hiddenTeams[i];
          var hidden = hiddenTeams[i];
          it.setAttribute('aria-pressed', hidden ? 'true' : 'false');
          d.style.opacity = hidden ? '0.25' : '';
          n.style.opacity = hidden ? '0.25' : '';
          n.style.textDecoration = hidden ? 'line-through' : '';

          if (hidden && hovIdx === i) hovIdx = null;
          applyDim();
        }

        it.addEventListener('click', toggleHidden);
        it.addEventListener('keydown', function (e) {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleHidden(); }
        });
        it.addEventListener('mouseenter', function () {
          if (hiddenTeams[i]) return;
          hovIdx = i; applyDim();
        });
        it.addEventListener('mouseleave', function () {
          hovIdx = null; applyDim();
        });
      }(idx, item, dot, nm));

      legend.appendChild(item);
    });
    container.appendChild(legend);
  }

  function showChartSkeleton(container) {
    var skel = document.createElement('div');
    skel.className = 'chart-skeleton';
    for (var i = 0; i < 8; i++) {
      var ln = document.createElement('div');
      ln.className = 'chart-skeleton-line';
      skel.appendChild(ln);
    }
    container.appendChild(skel);
  }

  function initChart() {
    var container = document.getElementById('chart-container');
    if (!container) return;

    showChartSkeleton(container);

    var chartData = null;

    fetch('/api/chart')
      .then(function (r) { return r.json(); })
      .then(function (d) {
        chartData = d;
        container.innerHTML = '';
        buildChart(d, container);
      })
      .catch(function () {
        container.innerHTML = '';
        showPlaceholder(container, 'Не удалось загрузить данные');
      });

    var resizeTimer = null;
    var lastW = container.clientWidth;
    window.addEventListener('resize', function () {
      if (!chartData) return;
      if (resizeTimer) clearTimeout(resizeTimer);
      resizeTimer = setTimeout(function () {
        if (container.clientWidth === lastW) return;
        lastW = container.clientWidth;
        buildChart(chartData, container);
      }, 150);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initChart);
  } else {
    initChart();
  }
}());
