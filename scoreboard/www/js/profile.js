(function () {
  'use strict';

  var proto = location.protocol === 'https:' ? 'wss://' : 'ws://';

  var info = new WebSocket(proto + location.host + '/info');
  info.onmessage = function (e) {
    var el = document.getElementById('info');
    if (el) el.innerHTML = e.data;
  };

  var READ_KEY = 'etalon_news_read';

  function readSet() {
    try {
      return JSON.parse(localStorage.getItem(READ_KEY)) || {};
    } catch (e) {
      return {};
    }
  }

  function markRead(id) {
    var s = readSet();
    if (s[id]) return;
    s[id] = 1;
    try {
      localStorage.setItem(READ_KEY, JSON.stringify(s));
    } catch (e) {}
  }

  function initAccordion() {
    var items = document.querySelectorAll('.news-acc-item');
    var read = readSet();

    items.forEach(function (item) {
      var trigger = item.querySelector('.news-acc-trigger');
      var body = item.querySelector('.news-acc-body');
      if (!trigger || !body) return;

      var newsID = item.getAttribute('data-news-id');
      var dot = item.querySelector('.news-acc-unread');
      if (newsID && dot && read[newsID]) {
        dot.remove();
        dot = null;
      }

      trigger.setAttribute('aria-expanded', 'false');
      trigger.addEventListener('click', function () {
        var isOpen = item.classList.contains('open');

        items.forEach(function (other) {
          if (other !== item) {
            other.classList.remove('open');
            var ot = other.querySelector('.news-acc-trigger');
            if (ot) ot.setAttribute('aria-expanded', 'false');
          }
        });

        item.classList.toggle('open', !isOpen);
        trigger.setAttribute('aria-expanded', isOpen ? 'false' : 'true');

        if (!isOpen && newsID) {
          markRead(newsID);
          if (dot) {
            dot.remove();
            dot = null;
          }
        }
      });

      trigger.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          trigger.click();
        }
      });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initAccordion);
  } else {
    initAccordion();
  }

}());
