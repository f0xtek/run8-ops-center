/* Applies the saved colour theme before first paint. Without a saved choice the
   stylesheet follows prefers-color-scheme. */
(function () {
  "use strict";
  try {
    var t = localStorage.getItem("run8ops-theme");
    if (t === "light" || t === "dark") {
      document.documentElement.setAttribute("data-theme", t);
    }
  } catch (e) { /* storage blocked: stay on the system theme */ }
})();
