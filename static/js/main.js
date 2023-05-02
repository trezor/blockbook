function syntaxHighlight(json) {
  json = JSON.stringify(json, undefined, 2);
  json = json
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
  if (json.length > 1000000) {
    return `<span class="key">${json}</span>`;
  }
  return json.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
    (match) => {
      let cls = "number";
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          cls = "key";
        } else {
          cls = "string";
        }
      } else if (/true|false/.test(match)) {
        cls = "boolean";
      } else if (/null/.test(match)) {
        cls = "null";
      }
      return `<span class="${cls}">${match}</span>`;
    }
  );
}

function getCoinCookie() {
  if(hasSecondary) return document.cookie
  .split("; ")
  .find((row) => row.startsWith("secondary_coin="))
  ?.split("=");
}

function changeCSSStyle(selector, cssProp, cssVal) {
  const mIndex = 1;
  const cssRules = document.all ? "rules" : "cssRules";
  for (
    i = 0, len = document.styleSheets[mIndex][cssRules].length;
    i < len;
    i++
  ) {
    if (document.styleSheets[mIndex][cssRules][i].selectorText === selector) {
      document.styleSheets[mIndex][cssRules][i].style[cssProp] = cssVal;
      return;
    }
  }
}

function amountTooltip() {
  const prim = this.querySelector(".prim-amt");
  const sec = this.querySelector(".sec-amt");
  const csec = this.querySelector(".csec-amt");
  const base = this.querySelector(".base-amt");
  const cbase = this.querySelector(".cbase-amt");
  let s = `${prim.outerHTML}<br>`;
  if (base) {
    let t = base.getAttribute("tm");
    if (!t) {
      t = "now";
    }
    s += `<span class="amt-time">${t}</span>${base.outerHTML}<br>`;
  }
  if (cbase) {
    s += `<span class="amt-time">now</span>${cbase.outerHTML}<br>`;
  }
  if (sec) {
    let t = sec.getAttribute("tm");
    if (!t) {
      t = "now";
    }
    s += `<span class="amt-time">${t}</span>${sec.outerHTML}<br>`;
  }
  if (csec) {
    s += `<span class="amt-time">now</span>${csec.outerHTML}<br>`;
  }
  return `<span class="l-tooltip">${s}</span>`;
}

function addressAliasTooltip() {
  const type = this.getAttribute("alias-type");
  const address = this.getAttribute("cc");
  return `<span class="l-tooltip">${type}<br>${address}</span>`;
}

window.addEventListener("DOMContentLoaded", () => {
  const a = getCoinCookie();
  if (a?.length === 3) {
    if (a[2] === "true") {
      changeCSSStyle(".prim-amt", "display", "none");
      changeCSSStyle(".sec-amt", "display", "initial");
    }
    document
      .querySelectorAll(".amt")
      .forEach(
        (e) => new bootstrap.Tooltip(e, { title: amountTooltip, html: true })
      );
  }

  document
    .querySelectorAll("[alias-type]")
    .forEach(
      (e) =>
        new bootstrap.Tooltip(e, { title: addressAliasTooltip, html: true })
    );

  document
    .querySelectorAll("[tt]")
    .forEach((e) => new bootstrap.Tooltip(e, { title: e.getAttribute("tt") }));

  document.querySelectorAll("#header .bb-group>.btn-check").forEach((e) =>
    e.addEventListener("click", (e) => {
      const a = getCoinCookie();
      const sc = e.target.id === "secondary-coin";
      if (a?.length === 3 && (a[2] === "true") !== sc) {
        document.cookie = `${a[0]}=${a[1]}=${sc}; Path=/`;
        changeCSSStyle(".prim-amt", "display", sc ? "none" : "initial");
        changeCSSStyle(".sec-amt", "display", sc ? "initial" : "none");
      }
    })
  );

  document.querySelectorAll(".copyable").forEach((e) =>
    e.addEventListener("click", (e) => {
      if (e.clientX < e.target.getBoundingClientRect().x) {
        let t = e.target.getAttribute("cc");
        if (!t) t = e.target.innerText;
        navigator.clipboard.writeText(t);
        e.target.className = e.target.className.replace("copyable", "copied");
        setTimeout(
          () =>
            (e.target.className = e.target.className.replace(
              "copied",
              "copyable"
            )),
          1000
        );
        e.preventDefault();
      }
    })
  );
});
