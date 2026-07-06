package server

// controlUIHTML is the embedded single-page control interface.
// Dependency-free vanilla HTML/JS; the access token is entered by the user
// and kept in localStorage — the page itself contains no secrets.
const controlUIHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>EmberClaw Control</title>
<style>
  :root { --bg:#101418; --panel:#1a2028; --border:#2a323d; --text:#e6edf3; --dim:#8b98a5; --accent:#ff7a45; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--bg); color:var(--text); font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; display:flex; flex-direction:column; min-height:100vh; }
  header { padding:14px 20px; border-bottom:1px solid var(--border); display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
  header h1 { font-size:17px; margin:0; }
  header h1 span { color:var(--accent); }
  #status { color:var(--dim); font-size:13px; }
  #status b { color:var(--text); font-weight:600; }
  #tokenrow { margin-left:auto; display:flex; gap:8px; }
  input, button, textarea { font:inherit; color:var(--text); background:var(--panel); border:1px solid var(--border); border-radius:8px; padding:8px 10px; }
  input:focus, textarea:focus { outline:none; border-color:var(--accent); }
  button { cursor:pointer; background:var(--accent); border-color:var(--accent); color:#101418; font-weight:600; }
  button:disabled { opacity:.5; cursor:default; }
  main { flex:1; display:flex; flex-direction:column; max-width:860px; width:100%; margin:0 auto; padding:20px; gap:12px; }
  #log { flex:1; overflow-y:auto; display:flex; flex-direction:column; gap:10px; padding-bottom:8px; }
  .msg { padding:10px 14px; border-radius:10px; max-width:85%; white-space:pre-wrap; word-wrap:break-word; }
  .me  { background:#26405a; align-self:flex-end; }
  .bot { background:var(--panel); border:1px solid var(--border); align-self:flex-start; }
  .err { background:#4a2328; border:1px solid #7d3a41; align-self:flex-start; }
  .sys { color:var(--dim); font-size:13px; align-self:center; }
  #inputrow { display:flex; gap:8px; }
  #message { flex:1; resize:none; min-height:44px; max-height:160px; }
</style>
</head>
<body>
<header>
  <h1><span>&#9679;</span> EmberClaw Control</h1>
  <div id="status">connecting&hellip;</div>
  <div id="tokenrow">
    <input id="token" type="password" placeholder="access token" size="22">
    <button id="save">Connect</button>
  </div>
</header>
<main>
  <div id="log"><div class="sys">Enter the access token to connect. Chat requests can take a while when the agent runs tools.</div></div>
  <div id="inputrow">
    <textarea id="message" placeholder="Message the agent&hellip;" rows="1"></textarea>
    <button id="send">Send</button>
  </div>
</main>
<script>
(function(){
  var $ = function(id){ return document.getElementById(id); };
  var sessionId = null;
  $('token').value = localStorage.getItem('eclaw_token') || '';

  function add(cls, text){
    var d = document.createElement('div');
    d.className = 'msg ' + cls;
    d.textContent = text;
    $('log').appendChild(d);
    $('log').scrollTop = $('log').scrollHeight;
    return d;
  }
  function hdrs(){
    return { 'Authorization': 'Bearer ' + $('token').value, 'Content-Type': 'application/json' };
  }
  function refreshStatus(){
    if (!$('token').value) { $('status').textContent = 'no token'; return; }
    fetch('/api/status', { headers: hdrs() }).then(function(r){ return r.json(); }).then(function(s){
      if (s.error) { $('status').textContent = s.error; return; }
      var up = Math.floor(s.uptime_seconds/3600) + 'h' + Math.floor(s.uptime_seconds%3600/60) + 'm';
      $('status').innerHTML = '<b>' + s.model + '</b> via ' + s.provider + ' &middot; up ' + up + ' &middot; ' + (s.ready ? 'ready' : 'not ready');
    }).catch(function(){ $('status').textContent = 'unreachable'; });
  }
  function send(){
    var text = $('message').value.trim();
    if (!text) return;
    $('message').value = '';
    add('me', text);
    var pending = add('sys', 'thinking…');
    $('send').disabled = true;
    fetch('/api/chat', { method:'POST', headers: hdrs(), body: JSON.stringify({ message:text, session_id:sessionId }) })
      .then(function(r){ return r.json(); })
      .then(function(res){
        pending.remove();
        if (res.error) { add('err', res.error); return; }
        sessionId = res.session_id;
        add('bot', res.response);
      })
      .catch(function(e){ pending.remove(); add('err', 'request failed: ' + e); })
      .finally(function(){ $('send').disabled = false; $('message').focus(); });
  }
  $('save').onclick = function(){ localStorage.setItem('eclaw_token', $('token').value); refreshStatus(); };
  $('send').onclick = send;
  $('message').addEventListener('keydown', function(e){
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
  });
  refreshStatus();
  setInterval(refreshStatus, 30000);
})();
</script>
</body>
</html>
`
