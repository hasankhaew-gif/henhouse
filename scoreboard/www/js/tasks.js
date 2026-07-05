(function () {
  'use strict';

  var proto = location.protocol === 'https:' ? 'wss://' : 'ws://';

  var CAT_COLORS = {
    'web':         '#4a9eff',
    'pwn':         '#e05c5c',
    'crypto':      '#a78bfa',
    'cryptography':'#a78bfa',
    'rev':         '#4fd1a5',
    'reverse':     '#4fd1a5',
    'reversing':   '#4fd1a5',
    'forensics':   '#e8a94a',
    'for':         '#e8a94a',
    'misc':        '#9aa0a6',
    'osint':       '#c07abf',
    'hardware':    '#d4845a',
    'stego':       '#60c9d4',
    'binary':      '#e05c5c',
  };

  function catColor(name) {
    return CAT_COLORS[(name || '').toLowerCase().trim()] || '#f5a623';
  }

  function applyCategoriColors(container) {
    container.querySelectorAll('.category').forEach(function (catEl) {
      var nameEl = catEl.querySelector('.category-name');
      if (!nameEl) return;
      var color = catColor(nameEl.textContent);
      catEl.style.setProperty('--cat-color', color);
      var bar = catEl.querySelector('.category-bar');
      if (bar) bar.style.background = color;
    });
  }

  function makeSkeletonCard() {
    var card = document.createElement('div');
    card.className = 'skeleton-card';
    var hdr = document.createElement('div');
    hdr.className = 'skeleton-card-header';
    var hdrLine = document.createElement('div');
    hdrLine.className = 'skeleton';
    hdr.appendChild(hdrLine);
    var body = document.createElement('div');
    body.className = 'skeleton-card-body';
    var bodyLine = document.createElement('div');
    bodyLine.className = 'skeleton';
    body.appendChild(bodyLine);
    var foot = document.createElement('div');
    foot.className = 'skeleton-card-footer';
    var tagSkel = document.createElement('div');
    tagSkel.className = 'skeleton skeleton-tag';
    var solvesSkel = document.createElement('div');
    solvesSkel.className = 'skeleton skeleton-solves';
    foot.appendChild(tagSkel);
    foot.appendChild(solvesSkel);
    card.appendChild(hdr);
    card.appendChild(body);
    card.appendChild(foot);
    return card;
  }

  function showSummarySkeleton() {
    var el = document.getElementById('tasks-summary');
    if (!el || el.children.length > 0) return;
    var wrap = document.createElement('div');
    wrap.className = 'summary-skeleton';
    var left = document.createElement('div');
    left.className = 'summary-skeleton-left';
    var l1 = document.createElement('div'); l1.className = 'skeleton';
    var l2 = document.createElement('div'); l2.className = 'skeleton';
    left.appendChild(l1); left.appendChild(l2);
    var right = document.createElement('div');
    right.className = 'summary-skeleton-right';
    for (var i = 0; i < 2; i++) {
      var stat = document.createElement('div');
      stat.className = 'summary-skeleton-stat';
      var s1 = document.createElement('div'); s1.className = 'skeleton';
      var s2 = document.createElement('div'); s2.className = 'skeleton';
      stat.appendChild(s1); stat.appendChild(s2);
      right.appendChild(stat);
      if (i === 0) {
        var div = document.createElement('div');
        div.style.cssText = 'width:1px;height:38px;background:var(--border-2)';
        right.appendChild(div);
      }
    }
    wrap.appendChild(left);
    wrap.appendChild(right);
    el.appendChild(wrap);
  }

  function showTasksSkeleton() {
    var el = document.getElementById('tasks-table');
    if (!el || el.children.length > 0) return;
    ['web', 'pwn', 'crypto'].forEach(function (name, ci) {
      var counts = [4, 3, 4];
      var catDiv = document.createElement('div');
      catDiv.className = 'category';
      var hdr = document.createElement('div');
      hdr.className = 'category-header';
      var bar = document.createElement('span');
      bar.className = 'category-bar';
      bar.style.background = catColor(name);
      var nameSkel = document.createElement('span');
      nameSkel.className = 'skeleton';
      nameSkel.style.cssText = 'width:60px;height:12px;display:inline-block;';
      hdr.appendChild(bar);
      hdr.appendChild(nameSkel);
      var grid = document.createElement('div');
      grid.className = 'category-tasks';
      for (var i = 0; i < counts[ci]; i++) {
        grid.appendChild(makeSkeletonCard());
      }
      catDiv.appendChild(hdr);
      catDiv.appendChild(grid);
      el.appendChild(catDiv);
    });
  }

  function staggerReveal(container) {
    var cards = container.querySelectorAll('.task_block');
    cards.forEach(function (card, i) {
      card.classList.add('task-reveal');
      card.style.animationDelay = (i * 45) + 'ms';
    });
  }

  function solvesPhrase(n) {
    var mod10 = n % 10, mod100 = n % 100;
    if (n === 0) return 'Ещё никто не решил';
    if (mod10 === 1 && mod100 !== 11) return n + ' команда решила';
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return n + ' команды решили';
    return n + ' команд решили';
  }

  var modalOverlay = null;

  function closeModal() {
    if (modalOverlay) {
      modalOverlay.remove();
      modalOverlay = null;
      document.removeEventListener('keydown', onModalKey);
    }
  }

  function onModalKey(e) {
    if (e.key === 'Escape') closeModal();
  }

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }

  var FEATHER = {
    check: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"></polyline></svg>',
    x: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>'
  };

  function icon(name, className) {
    var span = el('span', className);
    span.innerHTML = FEATHER[name];
    return span;
  }

  function buildSolvedBanner(extra) {
    var banner = el('div', 'task-solved-banner');
    banner.appendChild(icon('check', 'task-solved-check'));
    var txt = el('div', '');
    txt.appendChild(el('div', 'task-solved-title', 'Решено вашей командой'));
    if (extra) txt.appendChild(el('div', 'task-solved-sub', extra));
    banner.appendChild(txt);
    return banner;
  }

  function buildFlagForm(task, modal) {
    var wrap = el('div', 'task-modal-flag');
    wrap.appendChild(el('div', 'flag-form-label', 'Отправить флаг'));

    var group = el('div', 'input-group');
    var input = el('input', 'form-control');
    input.name = 'flag';
    input.placeholder = 'Флаг';
    input.spellcheck = false;
    input.autocomplete = 'off';
    var btnWrap = el('span', 'input-group-btn');
    var btn = el('button', 'btn btn-submit', 'Отправить');
    btn.type = 'button';
    btnWrap.appendChild(btn);
    group.appendChild(input);
    group.appendChild(btnWrap);
    wrap.appendChild(group);

    var result = el('div', 'task-modal-result');
    result.style.display = 'none';
    wrap.appendChild(result);

    function submit() {
      var flag = input.value.trim();
      if (!flag || btn.disabled) return;
      btn.disabled = true;
      btn.textContent = 'Проверка...';
      result.style.display = 'none';

      fetch('/api/flag?id=' + task.id, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'flag=' + encodeURIComponent(flag)
      })
        .then(function (r) { return r.json(); })
        .then(function (d) {
          if (d.ok) {

            var banner = buildSolvedBanner('+' + task.price + ' очков вашей команде');
            wrap.replaceWith(banner);
            var header = modal.querySelector('.task-modal-header');
            if (header) {
              header.classList.add('solved');
              var catLabel = header.querySelector('.task-modal-cat');
              if (catLabel) catLabel.style.color = '';
            }
          } else {
            result.textContent = d.msg || 'Неверный флаг. Попробуйте ещё раз.';
            result.className = 'task-modal-result invalid';
            result.style.display = '';
            btn.disabled = false;
            btn.textContent = 'Отправить';
            input.select();
          }
        })
        .catch(function () {
          result.textContent = 'Ошибка соединения. Попробуйте ещё раз.';
          result.className = 'task-modal-result invalid';
          result.style.display = '';
          btn.disabled = false;
          btn.textContent = 'Отправить';
        });
    }

    btn.addEventListener('click', submit);
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); submit(); }
    });

    return wrap;
  }

  function openModal(task) {
    closeModal();

    var color = catColor(task.cat);

    modalOverlay = el('div', 'task-modal-overlay');
    var modal = el('div', 'task-modal');

    var header = el('div', 'task-modal-header' + (task.solved ? ' solved' : ''));
    var headMeta = el('div', 'task-modal-header-meta');
    var catEl = el('span', 'task-modal-cat', task.cat);
    catEl.style.color = task.solved ? '' : color;
    headMeta.appendChild(catEl);
    headMeta.appendChild(el('span', 'task-modal-header-div'));
    headMeta.appendChild(el('span', 'task-modal-solves', solvesPhrase(task.solves)));
    header.appendChild(headMeta);
    var closeBtn = el('button', 'task-modal-close');
    closeBtn.innerHTML = FEATHER.x;
    closeBtn.type = 'button';
    closeBtn.setAttribute('aria-label', 'Закрыть');
    closeBtn.addEventListener('click', closeModal);
    header.appendChild(closeBtn);
    modal.appendChild(header);

    var body = el('div', 'task-modal-body');

    var titleRow = el('div', 'task-modal-title-row');
    var title = el('h2', 'task-modal-title', task.name);
    var priceBlock = el('div', 'task-header-price');
    var priceBig = el('div', 'task-price-big', '~' + task.price);
    var star = el('sup', 'task-price-star', '*');
    priceBig.appendChild(star);
    priceBlock.appendChild(priceBig);
    priceBlock.appendChild(el('div', 'task-price-label', 'очков'));
    titleRow.appendChild(title);
    titleRow.appendChild(priceBlock);
    body.appendChild(titleRow);

    var desc = el('div', 'task-modal-desc');
    desc.innerHTML = task.desc;
    body.appendChild(desc);

    if (task.files && task.files.length) {
      var filesBox = el('div', 'task-files');
      filesBox.appendChild(el('div', 'task-files-label', 'Файлы'));
      task.files.forEach(function (f) {
        var a = el('a', 'task-file-link', f.name);
        a.href = f.url;
        a.setAttribute('download', '');
        filesBox.appendChild(a);
      });
      body.appendChild(filesBox);
    }

    if (task.author) {
      body.appendChild(el('div', 'task-author', 'Автор: ' + task.author));
    }

    if (task.solved) {
      body.appendChild(buildSolvedBanner());
    } else {
      body.appendChild(buildFlagForm(task, modal));
    }

    modal.appendChild(body);
    modalOverlay.appendChild(modal);

    modalOverlay.addEventListener('click', function (e) {
      if (e.target === modalOverlay) closeModal();
    });
    document.addEventListener('keydown', onModalKey);

    document.body.appendChild(modalOverlay);

    var input = modal.querySelector('.form-control');
    if (input) input.focus();
  }

  function initModalDelegation() {
    var container = document.getElementById('tasks-table');
    if (!container) return;
    container.addEventListener('click', function (e) {
      var card = e.target.closest('a.task_block');
      if (!card || !card.getAttribute('href')) return;
      var m = card.getAttribute('href').match(/id=(\d+)/);
      if (!m) return;
      e.preventDefault();

      fetch('/api/task?id=' + m[1])
        .then(function (r) {
          if (!r.ok) throw new Error('bad status');
          return r.json();
        })
        .then(openModal)
        .catch(function () {

          location.href = card.getAttribute('href');
        });
    });
  }

  var firstWS = true;

  var tasksWS = new WebSocket(proto + location.host + '/tasks');
  tasksWS.onmessage = function (e) {
    var el = document.getElementById('tasks-table');
    if (!el) return;
    el.innerHTML = e.data;
    applyCategoriColors(el);
    if (firstWS) {
      firstWS = false;
      staggerReveal(el);
    }
  };

  var summaryWS = new WebSocket(proto + location.host + '/tasks-summary');
  summaryWS.onmessage = function (e) {
    var el = document.getElementById('tasks-summary');
    if (el) el.innerHTML = e.data;
  };

  var infoWS = new WebSocket(proto + location.host + '/info');
  infoWS.onmessage = function (e) {
    var el = document.getElementById('info');
    if (el) el.innerHTML = e.data;
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () {
      showSummarySkeleton();
      showTasksSkeleton();
      initModalDelegation();
    });
  } else {
    showSummarySkeleton();
    showTasksSkeleton();
    initModalDelegation();
  }

}());
