/* Run8 Operations Centre: live fragment refresh and async form helper.
 *
 * - Subscribes to /events (Server-Sent Events). On "view-changed" it refetches the
 *   current page with fragment=1 and swaps #main, plus the header and tab bar.
 * - Does not swap while the user is typing in a form field; it swaps when the field
 *   loses focus.
 * - Forms marked data-async submit with fetch and swap the fragment the server returns.
 * - No network requests leave this origin.
 */
(function () {
  "use strict";

  var root = document.documentElement;

  function mainEl() { return document.getElementById("main"); }

  /* ---------- toast ---------- */
  var toastTimer = 0;
  function toast(message) {
    var el = document.getElementById("toast");
    if (!el || !message) { return; }
    el.textContent = message;
    el.classList.remove("hidden");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { el.classList.add("hidden"); }, 4000);
  }

  function headerText(res, name) {
    var v = res.headers.get(name);
    if (!v) { return ""; }
    try { return decodeURIComponent(v); } catch (e) { return v; }
  }

  /* ---------- user-input guard ---------- */
  var TEXT_INPUT = "textarea,select,[contenteditable=''],[contenteditable='true']," +
    "input:not([type=checkbox]):not([type=radio]):not([type=button]):not([type=submit])" +
    ":not([type=reset]):not([type=range]):not([type=color]):not([type=file])";

  function userIsTyping() {
    var a = document.activeElement, m = mainEl();
    return !!(a && m && m !== a && m.contains(a) && a.matches(TEXT_INPUT));
  }

  /* ---------- swapping ---------- */
  function fragmentURL(path) {
    var u = new URL(path || location.pathname + location.search, location.origin);
    u.searchParams.set("fragment", "1");
    return u.pathname + u.search;
  }

  function fetchOK(url, init) {
    init = init || {};
    init.cache = "no-store";
    init.credentials = "same-origin";
    init.headers = Object.assign({ "X-Fragment": "1" }, init.headers || {});
    return fetch(url, init);
  }

  /* Replace the sticky header and tab bar with fresh server-rendered markup. */
  function swapChrome(html) {
    var tpl = document.createElement("template");
    tpl.innerHTML = html;
    ["header", "nav"].forEach(function (tag) {
      var fresh = tpl.content.querySelector(tag), old = document.querySelector(tag);
      if (fresh && old) { old.replaceWith(fresh); }
    });
  }

  function swapMain(html) {
    var m = mainEl();
    if (!m || m.innerHTML === html) { return; }
    var x = window.scrollX, y = window.scrollY;
    m.innerHTML = html;
    window.scrollTo(x, y);
  }

  function refreshChrome() {
    return fetchOK("/_chrome?path=" + encodeURIComponent(location.pathname))
      .then(function (res) { return res.ok ? res.text() : null; })
      .then(function (html) { if (html) { swapChrome(html); } })
      .catch(function () { /* the next event retries */ });
  }

  var inFlight = false, again = false;

  /* Refetch the current page's main fragment. Deferred while the user is typing. */
  function refresh() {
    if (userIsTyping()) { again = true; return Promise.resolve(); }
    if (inFlight) { again = true; return Promise.resolve(); }
    inFlight = true;
    again = false;
    var wanted = location.pathname + location.search;
    return Promise.all([
      fetchOK(fragmentURL()).then(function (res) { return res.ok ? res.text() : null; }),
      refreshChrome()
    ]).then(function (r) {
      /* Drop the result if the user navigated or began typing while it was in flight. */
      if (r[0] !== null && wanted === location.pathname + location.search && !userIsTyping()) {
        swapMain(r[0]);
      } else if (r[0] !== null) {
        again = true;
      }
    }).catch(function () { /* offline: the SSE reconnect refreshes */ })
      .then(function () {
        inFlight = false;
        if (again && !userIsTyping()) { refresh(); }
      });
  }

  document.addEventListener("focusout", function () {
    if (!again) { return; }
    /* Wait a tick: focus may be moving to another field. */
    setTimeout(function () { if (again && !userIsTyping()) { refresh(); } }, 0);
  });

  /* ---------- async forms ---------- */
  function submitForm(form, submitter) {
    var method = (form.getAttribute("method") || "GET").toUpperCase();
    var action = form.getAttribute("action") || location.pathname + location.search;
    var init = { method: method };
    var data = new FormData(form, submitter || undefined);
    if (method === "GET") {
      var u = new URL(action, location.href);
      new URLSearchParams(data).forEach(function (v, k) { u.searchParams.append(k, v); });
      action = u.pathname + u.search;
    } else {
      init.body = new URLSearchParams(data);
    }
    var buttons = form.querySelectorAll("button");
    buttons.forEach(function (b) { b.disabled = true; });
    return fetchOK(action, init).then(function (res) {
      var msg = headerText(res, "X-Toast");
      if (!res.ok) {
        return res.text().then(function (t) {
          toast(t.replace(/<[^>]*>/g, " ").trim().slice(0, 200) || (res.status + " " + res.statusText));
        });
      }
      /* A redirect moves the page: res.url is where fetch ended up. A direct answer
         to the POST leaves the user where they are. */
      var settled = new URL(res.url, location.href);
      settled.searchParams.delete("fragment");
      var target = settled.pathname + settled.search + settled.hash;
      var title = headerText(res, "X-Page-Title");
      return res.text().then(function (html) {
        if (res.redirected && settled.pathname + settled.search !== location.pathname + location.search) {
          history.pushState(null, "", target);
        }
        if (title && res.redirected) { document.title = title; }
        swapMain(html);
        if (msg) { toast(msg); }
        return refreshChrome();
      });
    }).catch(function () {
      toast("Request failed. Is the app still running?");
    }).then(function () {
      buttons.forEach(function (b) { b.disabled = false; });
    });
  }

  document.addEventListener("submit", function (ev) {
    var form = ev.target;
    if (!(form instanceof HTMLFormElement) || !form.hasAttribute("data-async")) { return; }
    var q = form.getAttribute("data-confirm");
    ev.preventDefault();
    if (q && !window.confirm(q)) { return; }
    submitForm(form, ev.submitter);
  });

  window.addEventListener("popstate", function () { refresh(); });

  /* ---------- theme toggle ---------- */
  document.addEventListener("click", function (ev) {
    var t = ev.target instanceof Element ? ev.target.closest("#theme-toggle") : null;
    if (!t) { return; }
    var cur = root.getAttribute("data-theme");
    if (!cur) {
      cur = window.matchMedia && matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    var next = cur === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("run8ops-theme", next); } catch (e) { /* ignore */ }
  });

  /* ---------- server-sent events ---------- */
  function connect() {
    if (!window.EventSource) { return; }
    var es = new EventSource("/events");
    var wasDown = false;
    es.addEventListener("open", function () {
      root.setAttribute("data-live", "on");
      /* Events may have fired while the stream was down. */
      if (wasDown) { wasDown = false; refresh(); }
    });
    es.addEventListener("view-changed", function () { refresh(); });
    es.addEventListener("error", function () {
      root.setAttribute("data-live", "off");
      wasDown = true;
    });
  }

  window.Run8 = { refresh: refresh, submitForm: submitForm, toast: toast };
  connect();
})();
