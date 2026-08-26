// Runs synchronously in <head>, before first paint: a deferred or async
// script would let the page paint the browser default theme first, then
// flip, which is the white flash this exists to avoid.
(function () {
  'use strict';

  // State is "auto", "light" or "dark". "auto" stores nothing.
  var STORAGE_KEY = 'ph-ui-theme';
  var root = document.documentElement;
  var media = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)');

  function readStored() {
    try {
      var v = window.localStorage.getItem(STORAGE_KEY);
      return v === 'light' || v === 'dark' ? v : 'auto';
    } catch (e) {
      return 'auto';
    }
  }

  function writeStored(mode) {
    try {
      if (mode === 'auto') {
        window.localStorage.removeItem(STORAGE_KEY);
      } else {
        window.localStorage.setItem(STORAGE_KEY, mode);
      }
    } catch (e) {
      // Private mode or blocked site data: the choice just won't survive a
      // reload. Nothing else to do about it here.
    }
  }

  function apply(mode) {
    if (mode === 'auto') {
      root.removeAttribute('data-theme');
    } else {
      root.setAttribute('data-theme', mode);
    }
  }

  var current = readStored();
  apply(current);

  // In auto, the OS can flip live (a machine switching at sunset) and the
  // page should follow without a reload. Nothing to do on the listener side
  // for a manual pin: the CSS itself only reacts to prefers-color-scheme
  // under the :not([data-theme="light"]) guard, which a pin already escapes.
  if (media && media.addEventListener) {
    media.addEventListener('change', function () {
      if (readStored() === 'auto') {
        root.removeAttribute('data-theme');
      }
    });
  }

  // The rail is restored here too, before first paint, or a collapsed rail
  // would visibly snap shut once this script's later DOMContentLoaded work
  // runs, the same flash class the theme logic above exists to avoid.
  var RAIL_KEY = 'ph-ui-rail-collapsed';

  function readRailCollapsed() {
    try {
      return window.localStorage.getItem(RAIL_KEY) === '1';
    } catch (e) {
      return false;
    }
  }

  function writeRailCollapsed(collapsed) {
    try {
      if (collapsed) {
        window.localStorage.setItem(RAIL_KEY, '1');
      } else {
        window.localStorage.removeItem(RAIL_KEY);
      }
    } catch (e) {
      // Same fallback as the theme: the choice just won't survive a reload.
    }
  }

  function applyRail(collapsed) {
    if (collapsed) {
      root.setAttribute('data-rail-collapsed', 'true');
    } else {
      root.removeAttribute('data-rail-collapsed');
    }
  }

  var railCollapsed = readRailCollapsed();
  applyRail(railCollapsed);

  // Same flash concern as the theme and the collapse state: restore the
  // dragged width before first paint, or the rail visibly snaps to it once
  // app.js's storage read runs later.
  // These four constants and clampRailWidth are duplicated in app.js, which
  // owns the live drag behaviour. Keep both in sync.
  var RAIL_WIDTH_KEY = 'ph-ui-rail-width';
  var RAIL_WIDTH_DEFAULT = 280;
  var RAIL_WIDTH_MIN = 200;
  var RAIL_WIDTH_MAX_RATIO = 0.45;

  function clampRailWidth(width) {
    var max = window.innerWidth * RAIL_WIDTH_MAX_RATIO;
    return Math.min(Math.max(width, RAIL_WIDTH_MIN), max);
  }

  function readRailWidth() {
    try {
      var raw = window.localStorage.getItem(RAIL_WIDTH_KEY);
      var value = raw === null ? NaN : parseFloat(raw);
      return Number.isFinite(value) ? clampRailWidth(value) : RAIL_WIDTH_DEFAULT;
    } catch (e) {
      return RAIL_WIDTH_DEFAULT;
    }
  }

  root.style.setProperty('--rail-width', readRailWidth() + 'px');

  function updateRailButton(btn) {
    btn.textContent = railCollapsed ? '⟩' : '⟨';
    var title = railCollapsed ? 'Expand component tree' : 'Collapse component tree';
    btn.setAttribute('title', title);
    btn.setAttribute('aria-label', title);
    btn.setAttribute('aria-expanded', railCollapsed ? 'false' : 'true');
  }

  function wireRailButton() {
    var btn = document.getElementById('rail-toggle');
    if (!btn) {
      return;
    }
    updateRailButton(btn);
    btn.addEventListener('click', function () {
      railCollapsed = !railCollapsed;
      writeRailCollapsed(railCollapsed);
      applyRail(railCollapsed);
      updateRailButton(btn);
    });
  }

  var NEXT = { auto: 'light', light: 'dark', dark: 'auto' };
  var ICON = {
    auto: '◐',
    light: '☀',
    dark: '☽'
  };
  var LABEL = { auto: 'Auto', light: 'Light', dark: 'Dark' };

  function updateButton(btn, mode) {
    btn.textContent = ICON[mode];
    btn.setAttribute('data-theme-state', mode);
    btn.setAttribute('aria-label', 'Theme: ' + LABEL[mode] + '. Click for ' + LABEL[NEXT[mode]] + '.');
    btn.setAttribute('title', 'Theme: ' + LABEL[mode] + ' (click for ' + LABEL[NEXT[mode]] + ')');
  }

  function wireButton() {
    var btn = document.getElementById('theme-toggle');
    if (!btn) {
      return;
    }
    updateButton(btn, current);
    btn.addEventListener('click', function () {
      current = NEXT[current];
      writeStored(current);
      apply(current);
      updateButton(btn, current);
    });
  }

  function wireControls() {
    wireButton();
    wireRailButton();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireControls);
  } else {
    wireControls();
  }
})();
