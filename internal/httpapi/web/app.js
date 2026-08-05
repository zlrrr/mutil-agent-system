// Console behaviour. No framework and no build step: the page is server-rendered and
// this file only keeps the timeline live and submits the two forms as JSON (ADR-005).
(function () {
  "use strict";

  // Open an investigation from the fault case list.
  document.querySelectorAll("form.js-open").forEach(function (form) {
    form.addEventListener("submit", async function (ev) {
      ev.preventDefault();
      const button = form.querySelector("button");
      button.disabled = true;
      button.textContent = "Running…";
      try {
        const res = await fetch("/api/catalog/cases");
        const cases = await res.json();
        const target = cases.find(function (c) { return c.id === form.dataset.case; });
        if (!target) throw new Error("unknown case " + form.dataset.case);

        const created = await fetch("/api/cases", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ case_ref: target.id }),
        });
        const body = await created.json();
        if (!created.ok) throw new Error(body.error || created.statusText);
        window.location = "/cases/" + body.id;
      } catch (err) {
        button.disabled = false;
        button.textContent = "Open investigation";
        alert("Could not open the investigation: " + err.message);
      }
    });
  });

  // Submit an approval decision.
  document.querySelectorAll("form.approval").forEach(function (form) {
    form.addEventListener("submit", async function (ev) {
      ev.preventDefault();
      const decision = ev.submitter ? ev.submitter.value : "approved";
      const data = new FormData(form);
      try {
        const res = await fetch(form.action, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            decision: decision,
            by: data.get("by") || "operator",
            comment: data.get("comment") || "",
          }),
        });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        window.location.reload();
      } catch (err) {
        alert("The decision was not recorded: " + err.message);
      }
    });
  });

  // Keep the timeline live. The stream resumes from the highest sequence already
  // rendered, so a reconnect neither duplicates nor drops a row.
  const timeline = document.getElementById("timeline");
  if (!timeline) return;
  const caseID = window.location.pathname.split("/").pop();
  let highest = 0;
  timeline.querySelectorAll("li[data-seq]").forEach(function (li) {
    highest = Math.max(highest, parseInt(li.dataset.seq, 10) || 0);
  });

  const source = new EventSource("/api/cases/" + caseID + "/stream?from=" + highest);
  source.addEventListener("case", function (msg) {
    let e;
    try { e = JSON.parse(msg.data); } catch (err) { return; }
    if (!e || e.seq <= highest) return;
    highest = e.seq;

    const li = document.createElement("li");
    li.dataset.seq = e.seq;
    li.innerHTML =
      '<span class="t-seq mono"></span><span class="t-actor"></span>' +
      '<span class="t-type mono small"></span><span class="t-summary"></span>';
    li.children[0].textContent = e.seq;
    li.children[1].textContent = e.actor;
    li.children[1].classList.add("actor-" + e.actor);
    li.children[2].textContent = e.type;
    li.children[3].textContent = e.summary;
    timeline.appendChild(li);
    timeline.scrollTop = timeline.scrollHeight;

    if (e.type === "state_changed") {
      const badge = document.getElementById("case-status");
      try {
        const to = JSON.parse(atob(e.payload)).to;
        if (badge && to) { badge.textContent = to; badge.className = "pill status-" + to; }
      } catch (err) { /* payload shape is advisory for the badge only */ }
    }
    if (e.type === "case_closed") { source.close(); window.location.reload(); }
  });
  source.onerror = function () { /* the browser retries; the cursor prevents loss */ };
})();
