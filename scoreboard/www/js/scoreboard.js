var proto = location.protocol === 'https:' ? 'wss://' : 'ws://';

var scoreboard = new WebSocket(proto + location.host + '/scoreboard');
scoreboard.onmessage = function (e) {
  var el = document.getElementById('scoreboard-table');
  if (el) el.innerHTML = e.data;
};

var info = new WebSocket(proto + location.host + '/info');
info.onmessage = function (e) {
  var el = document.getElementById('info');
  if (el) el.innerHTML = e.data;
};
