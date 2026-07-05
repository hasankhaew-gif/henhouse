(function () {
  'use strict';

  var text = document.getElementById('s-text');
  var count = document.getElementById('s-count');
  var counter = document.getElementById('s-counter');
  if (text && count && counter) {
    var update = function () {
      var len = text.value.length;
      count.textContent = String(len);
      counter.classList.remove('support-counter-warn', 'support-counter-max');
      if (len >= 2000) {
        counter.classList.add('support-counter-max');
      } else if (len >= 1800) {
        counter.classList.add('support-counter-warn');
      }
    };
    text.addEventListener('input', update);
    update();
  }

  var file = document.getElementById('s-file');
  var fileLabel = document.getElementById('s-file-label');
  if (file && fileLabel) {
    file.addEventListener('change', function () {
      if (file.files && file.files.length > 0) {
        fileLabel.textContent = file.files[0].name;
      } else {
        fileLabel.textContent = 'Прикрепить файл';
      }
    });
  }
})();
