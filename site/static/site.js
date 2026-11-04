/* Progressive enhancement. Every page works without this file. */
(function () {
  "use strict";

  var STORAGE_KEY = "u5r-hide-answers";

  function readPreference() {
    try {
      return window.localStorage.getItem(STORAGE_KEY) !== "0";
    } catch (error) {
      return true;
    }
  }

  function writePreference(hide) {
    try {
      window.localStorage.setItem(STORAGE_KEY, hide ? "1" : "0");
    } catch (error) {
      /* Storage can be blocked; the preference then lasts for this page only. */
    }
  }

  /* Spoilers: answers are in the HTML but hidden until revealed. */
  var hideAnswers = readPreference();

  function paintSpoiler(spoiler) {
    var answer = spoiler.querySelector(".answer");
    var button = spoiler.querySelector(".reveal");
    var open = !hideAnswers || spoiler.dataset.open === "true";
    answer.hidden = !open;
    if (hideAnswers) {
      button.hidden = false;
      button.textContent = open ? "Hide" : "Reveal";
      button.setAttribute("aria-expanded", open ? "true" : "false");
    } else {
      button.hidden = true;
    }
  }

  function paintAllSpoilers() {
    document.querySelectorAll(".spoiler").forEach(paintSpoiler);
    document.querySelectorAll("[data-hide-answers]").forEach(function (box) {
      box.checked = hideAnswers;
    });
  }

  document.querySelectorAll(".spoiler").forEach(function (spoiler) {
    spoiler.querySelector(".reveal").addEventListener("click", function () {
      spoiler.dataset.open = spoiler.dataset.open === "true" ? "false" : "true";
      paintSpoiler(spoiler);
    });
  });

  document.querySelectorAll("[data-hide-answers]").forEach(function (box) {
    box.addEventListener("change", function () {
      hideAnswers = box.checked;
      writePreference(hideAnswers);
      document.querySelectorAll(".spoiler").forEach(function (spoiler) {
        spoiler.dataset.open = "false";
      });
      paintAllSpoilers();
    });
  });
  paintAllSpoilers();

  /* Data tables: sort and filter using the published JSON. */
  function lookup(record, path) {
    return path.split(".").reduce(function (value, key) {
      return value === null || value === undefined ? value : value[key];
    }, record);
  }

  function compare(a, b) {
    if (a === b) {
      return 0;
    }
    if (a === null || a === undefined) {
      return 1;
    }
    if (b === null || b === undefined) {
      return -1;
    }
    if (typeof a === "number" && typeof b === "number") {
      return a - b;
    }
    return String(a).localeCompare(String(b), "en", { numeric: true });
  }

  function searchText(record) {
    return JSON.stringify(record).toLowerCase();
  }

  function enhanceTable(table) {
    fetch(table.dataset.json)
      .then(function (response) {
        return response.json();
      })
      .then(function (dataset) {
        var records = {};
        dataset.records.forEach(function (record) {
          records[String(record[dataset.key])] = record;
        });
        setupTable(table, records);
      })
      .catch(function () {
        /* The pre-rendered table stays as it is. */
      });
  }

  function setupTable(table, records) {
    var body = table.tBodies[0];
    var rows = Array.prototype.slice.call(body.rows);
    var original = rows.slice();
    var scope = table.closest("[data-table-scope]") || document;
    var state = {
      group: scope.dataset.defaultGroup || "all",
      query: "",
      key: null,
      direction: 1,
    };
    var count = scope.querySelector("[data-count]");

    function paint() {
      var visible = 0;
      rows.forEach(function (row) {
        var record = records[row.dataset.key];
        var show =
          (state.group === "all" || row.dataset.group === state.group) &&
          (state.query === "" || searchText(record).indexOf(state.query) !== -1);
        row.hidden = !show;
        visible += show ? 1 : 0;
      });
      if (count) {
        count.textContent = "Showing " + visible + " of " + rows.length;
      }
    }

    function sortBy(key) {
      state.direction = state.key === key ? -state.direction : 1;
      state.key = key;
      rows.sort(function (a, b) {
        return (
          state.direction *
          compare(lookup(records[a.dataset.key], key), lookup(records[b.dataset.key], key))
        );
      });
      rows.forEach(function (row) {
        body.appendChild(row);
      });
      table.querySelectorAll("th[data-sort-key]").forEach(function (th) {
        var active = th.dataset.sortKey === key;
        th.setAttribute("aria-sort", active ? (state.direction > 0 ? "ascending" : "descending") : "none");
      });
    }

    table.querySelectorAll("th[data-sort-key]").forEach(function (th) {
      var button = document.createElement("button");
      button.type = "button";
      button.textContent = th.textContent;
      th.textContent = "";
      th.appendChild(button);
      th.setAttribute("aria-sort", "none");
      button.addEventListener("click", function () {
        sortBy(th.dataset.sortKey);
      });
    });

    scope.querySelectorAll("[data-filter]").forEach(function (input) {
      input.hidden = false;
      input.addEventListener("input", function () {
        state.query = input.value.trim().toLowerCase();
        paint();
      });
    });

    var groupButtons = scope.querySelectorAll("[data-group]");
    groupButtons.forEach(function (button) {
      button.addEventListener("click", function () {
        state.group = button.dataset.group;
        groupButtons.forEach(function (other) {
          other.setAttribute("aria-pressed", other === button ? "true" : "false");
        });
        paint();
      });
    });

    scope.querySelectorAll("[data-reset-order]").forEach(function (button) {
      button.addEventListener("click", function () {
        state.key = null;
        original.forEach(function (row) {
          body.appendChild(row);
        });
        rows = original.slice();
        table.querySelectorAll("th[data-sort-key]").forEach(function (th) {
          th.setAttribute("aria-sort", "none");
        });
      });
    });

    scope.querySelectorAll("[data-js-only]").forEach(function (element) {
      element.hidden = false;
    });
    paint();
  }

  document.querySelectorAll("table[data-json]").forEach(enhanceTable);

  /* File catalog filter: hides rows (and empty groups) that do not match. */
  document.querySelectorAll("[data-row-filter]").forEach(function (input) {
    var table = document.getElementById(input.dataset.rowFilter);
    input.addEventListener("input", function () {
      var query = input.value.trim().toLowerCase();
      table.querySelectorAll("tbody").forEach(function (group) {
        var shown = 0;
        Array.prototype.slice.call(group.rows, 1).forEach(function (row) {
          var match = query === "" || row.textContent.toLowerCase().indexOf(query) !== -1;
          row.hidden = !match;
          shown += match ? 1 : 0;
        });
        group.hidden = shown === 0;
      });
    });
  });

  /* Myth verdict filter. */
  document.querySelectorAll("[data-verdict-filter]").forEach(function (button) {
    var buttons = document.querySelectorAll("[data-verdict-filter]");
    button.addEventListener("click", function () {
      var wanted = button.dataset.verdictFilter;
      buttons.forEach(function (other) {
        other.setAttribute("aria-pressed", other === button ? "true" : "false");
      });
      document.querySelectorAll("[data-verdict]").forEach(function (card) {
        card.hidden = wanted !== "all" && card.dataset.verdict !== wanted;
      });
    });
  });
  document.querySelectorAll("[data-js-only]").forEach(function (element) {
    element.hidden = false;
  });
})();
