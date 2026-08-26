// Client model, path keying, keyed reconciler and the rail's tree rendering.
// Tasks 12 and 13 add the graph and the detail dock on top of this file.
(function () {
  'use strict';

  var ROOT_PATH = '/';
  var COLLAPSE_THRESHOLD = 25;
  var EVENT_NAMES = ['snapshot', 'scan', 'scanning', 'scan-error', 'connection', 'shutdown'];

  // ---------------------------------------------------------------------
  // Pure functions: path keying, the tree walk, duration parsing and the
  // reconciler diff. No DOM access below this section.
  // ---------------------------------------------------------------------

  // Exactly two replacements, percent first, then slash: this must match the
  // server's PathKey byte for byte or transitions silently fail to match.
  function pathKey(parent, name) {
    var escaped = name.replace(/%/g, '%25').replace(/\//g, '%2F');
    return parent === '' ? escaped : parent + '/' + escaped;
  }

  // Walks the tree ourselves rather than via any flatten helper, so
  // satellite nodes and every Components entry stay in the index.
  function visitNode(node, parentActualPath, index) {
    var path;
    if (node && node.name) {
      var pkParent = (parentActualPath === null || parentActualPath === ROOT_PATH) ? '' : parentActualPath;
      path = pathKey(pkParent, node.name);
    } else if (parentActualPath === null) {
      path = ROOT_PATH;
    } else {
      path = parentActualPath;
    }

    var entry = { path: path, parentPath: parentActualPath, node: node || {}, childPaths: [] };
    index.set(path, entry);

    var children = (node && node.components) || [];
    for (var i = 0; i < children.length; i++) {
      entry.childPaths.push(visitNode(children[i], path, index));
    }
    return path;
  }

  function buildIndex(root) {
    var index = new Map();
    visitNode(root, null, index);
    return index;
  }

  function diffPaths(oldPathSet, newIndex) {
    var added = [];
    var removed = [];
    newIndex.forEach(function (entry, path) {
      if (!oldPathSet.has(path)) added.push(path);
    });
    oldPathSet.forEach(function (path) {
      if (!newIndex.has(path)) removed.push(path);
    });
    return { added: added, removed: removed };
  }

  function effectiveStatus(node) {
    return (node && node.status) || 'UNKNOWN';
  }

  function statusClass(status) {
    if (status === 'HEALTHY') return 'healthy';
    if (status === 'UNHEALTHY' || status === 'LOOP_DETECTED') return 'unhealthy';
    return 'unknown';
  }

  function isUnhealthy(status) {
    return status === 'UNHEALTHY' || status === 'LOOP_DETECTED';
  }

  // Render time only: filters, chips and the click-to-filter behaviour all
  // keep matching against the real type string. kubernetes is by far the
  // widest label and the most common one, so it alone drove the type
  // column's fixed width; shortening it is what let that column shrink.
  var TYPE_ABBREVIATIONS = { kubernetes: 'k8s', satellite: 'sat' };

  function displayType(type) {
    return TYPE_ABBREVIATIONS[type] || type;
  }

  // duration is the canonical proto3 string ("0.076347145s", "5s"), never a
  // number and never Go's "76ms". Strip the trailing "s" and parseFloat.
  function parseDurationSeconds(value) {
    if (typeof value !== 'string' || value.charAt(value.length - 1) !== 's') {
      return null;
    }
    var seconds = parseFloat(value.slice(0, -1));
    return Number.isFinite(seconds) ? seconds : null;
  }

  function formatDuration(value) {
    var seconds = parseDurationSeconds(value);
    if (seconds === null) return '';
    if (seconds < 1) {
      var ms = seconds * 1000;
      var roundedMs = ms < 10 ? Math.round(ms * 100) / 100 : Math.round(ms);
      return roundedMs + 'ms';
    }
    if (seconds < 60) {
      return (Math.round(seconds * 100) / 100) + 's';
    }
    var minutes = Math.floor(seconds / 60);
    var remainder = Math.round(seconds - minutes * 60);
    return minutes + 'm ' + remainder + 's';
  }

  function countUnhealthySubtreeInclusive(index, path) {
    var entry = index.get(path);
    if (!entry) return 0;
    var count = isUnhealthy(effectiveStatus(entry.node)) ? 1 : 0;
    for (var i = 0; i < entry.childPaths.length; i++) {
      count += countUnhealthySubtreeInclusive(index, entry.childPaths[i]);
    }
    return count;
  }

  function countUnhealthyDescendants(index, path) {
    var entry = index.get(path);
    if (!entry) return 0;
    var count = 0;
    for (var i = 0; i < entry.childPaths.length; i++) {
      count += countUnhealthySubtreeInclusive(index, entry.childPaths[i]);
    }
    return count;
  }

  function nodeMatches(node, viewState) {
    if (viewState.filter) {
      var name = (node.name || '').toLowerCase();
      if (name.indexOf(viewState.filter.toLowerCase()) === -1) return false;
    }
    if (viewState.typeFilter && node.type !== viewState.typeFilter) return false;
    if (viewState.unhealthyOnly && !isUnhealthy(effectiveStatus(node))) return false;
    return true;
  }

  // Pure function of (index, view): which paths are hidden entirely, and
  // which are open, given search, type and unhealthy filters plus the
  // persisted expand set. Ancestors of a match are shown as context, never
  // flattened, and a matching descendant forces its collapsed ancestor open
  // without mutating the persisted expand state.
  function computeVisibility(index, viewState) {
    var filterActive = Boolean(viewState.filter) || Boolean(viewState.typeFilter) || Boolean(viewState.unhealthyOnly);
    var match = new Map();

    function evaluate(path) {
      var cached = match.get(path);
      if (cached) return cached;
      var entry = index.get(path);
      var self = !filterActive || nodeMatches(entry.node, viewState);
      var subtree = self;
      for (var i = 0; i < entry.childPaths.length; i++) {
        if (evaluate(entry.childPaths[i]).subtree) subtree = true;
      }
      var result = { self: self, subtree: subtree };
      match.set(path, result);
      return result;
    }

    index.forEach(function (entry, path) { evaluate(path); });

    var visibility = new Map();
    index.forEach(function (entry, path) {
      if (path === ROOT_PATH) return;
      var hasChildren = entry.childPaths.length > 0;
      var hidden = filterActive && !match.get(path).subtree;
      var descendantMatches = hasChildren && entry.childPaths.some(function (child) {
        return match.get(child).subtree;
      });
      var forcedOpen = filterActive && descendantMatches;
      var open = hasChildren && (forcedOpen || viewState.expanded.has(path));
      visibility.set(path, { hidden: hidden, open: open, hasChildren: hasChildren });
    });
    return visibility;
  }

  // ---------------------------------------------------------------------
  // Detail, dock and banner descriptions. Pure: model in, render specs out.
  // Dispatch is on the "@type" suffix, mirroring how
  // pkg/platform_health/details dispatches server side, so a type this file
  // has no case for still renders as readable label and value pairs rather
  // than vanishing or throwing.
  // ---------------------------------------------------------------------

  function hasSuffix(value, suffix) {
    return value.length >= suffix.length && value.slice(value.length - suffix.length) === suffix;
  }

  function detailTypeName(detail) {
    var url = (detail && detail['@type']) || '';
    var slash = url.lastIndexOf('/');
    return slash >= 0 ? url.slice(slash + 1) : url;
  }

  function detailShortName(typeName) {
    var dot = typeName.lastIndexOf('.');
    var leaf = dot >= 0 ? typeName.slice(dot + 1) : typeName;
    return leaf.indexOf('Detail_') === 0 ? leaf.slice('Detail_'.length) : leaf;
  }

  function compact(list) {
    var out = [];
    for (var i = 0; i < list.length; i++) {
      if (list[i]) out.push(list[i]);
    }
    return out;
  }

  function field(key, label, value, tone) {
    if (value === undefined || value === null || value === '') return null;
    return { key: key, label: label, value: String(value), tone: tone || '' };
  }

  function crumbField(key, label, crumbs) {
    return crumbs.length ? { key: key, label: label, crumbs: crumbs, value: '', tone: '' } : null;
  }

  function group(key, label, rows) {
    return rows.length ? { key: key, label: label, rows: rows } : null;
  }

  function textRow(key, text, tone) {
    return { key: key, text: text, tone: tone || '' };
  }

  // Same wording as details.FormatDuration server side, so the same
  // certificate reads the same way in the dashboard and in ph check.
  function formatRemaining(ms) {
    if (ms < 0) return 'expired';
    var days = Math.floor(ms / 86400000);
    var hours = Math.floor(ms / 3600000);
    var minutes = Math.floor(ms / 60000);
    if (days > 30) return days + 'd remaining';
    if (days > 0) return days + 'd ' + (hours % 24) + 'h remaining';
    if (hours >= 1) return hours + 'h ' + (minutes % 60) + 'm remaining';
    if (minutes >= 1) return minutes + 'm remaining';
    return Math.floor(ms / 1000) + 's remaining';
  }

  function formatExpiry(value) {
    if (!value) return null;
    var when = new Date(value);
    if (Number.isNaN(when.getTime())) return null;
    var ms = when.getTime() - Date.now();
    var days = ms / 86400000;
    return {
      text: formatRemaining(ms),
      tone: ms < 0 ? 'error' : (days <= 30 ? 'warn' : 'ok')
    };
  }

  function describeTLS(d) {
    var expiry = formatExpiry(d.validUntil);
    var sans = d.subjectAltNames || [];
    var chain = d.chain || [];
    return {
      fields: compact([
        field('cn', 'Common name', d.commonName),
        expiry ? field('valid', 'Valid until', d.validUntil + ' (' + expiry.text + ')', expiry.tone) : null,
        field('version', 'TLS version', d.version),
        field('protocol', 'Protocol', d.protocol),
        field('cipher', 'Cipher suite', d.cipherSuite),
        field('sig', 'Signature algorithm', d.signatureAlgorithm),
        field('pk', 'Public key algorithm', d.publicKeyAlgorithm)
      ]),
      groups: compact([
        group('sans', 'Subject alt names', sans.map(function (name, i) {
          return textRow('san' + i, name);
        })),
        group('chain', 'Certificate chain', chain.map(function (cert, i) {
          return textRow('cert' + i, '[' + i + '] ' + cert);
        }))
      ])
    };
  }

  // Conditions carry no tone. True is good on Available and bad on Degraded,
  // so a generic mapping would colour half of them backwards.
  function describeKStatus(d) {
    var conditions = d.conditions || [];
    return {
      fields: compact([
        field('status', 'Status', d.status),
        field('message', 'Message', d.message)
      ]),
      groups: compact([
        group('conditions', 'Conditions', conditions.map(function (c, i) {
          var line = (c.type || '?') + '=' + (c.status || '?');
          if (c.reason) line += ' (' + c.reason + ')';
          if (c.message) line += ': ' + c.message;
          return textRow('cond' + i, line);
        }))
      ])
    };
  }

  // ttl is shown because it is real answer data, but it counts down between
  // scans and must never enter a comparison that decides what changed.
  function dnsRecordLine(record) {
    var line = (record.type || '?') + '  ' + (record.value || '');
    if (record.target) line += '  target ' + record.target;
    if (record.port) line += '  port ' + record.port;
    if (record.priority) line += '  priority ' + record.priority;
    if (record.weight) line += '  weight ' + record.weight;
    if (record.ttl !== undefined && record.ttl !== null) line += '  ttl ' + record.ttl + 's';
    return line;
  }

  function describeDNS(d) {
    var records = d.records || [];
    var dnssec = d.dnssec || null;
    var dnssecText = '';
    if (dnssec) {
      dnssecText = (dnssec.enabled ? 'enabled' : 'not enabled') + ', ' +
        (dnssec.authenticated ? 'authenticated' : 'not authenticated');
    }
    return {
      fields: compact([
        field('host', 'Host', d.host),
        field('server', 'Resolver', d.server),
        field('query', 'Query type', d.queryType),
        field('dnssec', 'DNSSEC', dnssecText)
      ]),
      groups: compact([
        group('records', 'Answers', records.map(function (record, i) {
          return textRow('rr' + i, dnsRecordLine(record));
        }))
      ])
    };
  }

  function describeLoop(d) {
    var ids = d.serverIds || [];
    return {
      fields: [],
      groups: compact([
        group('chain', 'Server chain', [textRow('ids', ids.join(' -> '))])
      ])
    };
  }

  function safeJSON(value) {
    try {
      return JSON.stringify(value);
    } catch (e) {
      return '[unrenderable value]';
    }
  }

  function describeUnknownDetail(d) {
    var fields = [];
    Object.keys(d).forEach(function (key) {
      if (key === '@type') return;
      var value = d[key];
      var text = (value !== null && typeof value === 'object') ? safeJSON(value) : String(value);
      var entry = field(key, key, text);
      if (entry) fields.push(entry);
    });
    return { fields: fields, groups: [] };
  }

  function describeDetail(detail, position) {
    var typeName = detailTypeName(detail || {});
    var body;
    var known = true;
    if (!detail || typeof detail !== 'object') {
      body = { fields: [], groups: [] };
      known = false;
    } else if (hasSuffix(typeName, 'Detail_TLS')) {
      body = describeTLS(detail);
    } else if (hasSuffix(typeName, 'Detail_KStatus')) {
      body = describeKStatus(detail);
    } else if (hasSuffix(typeName, 'Detail_DNS')) {
      body = describeDNS(detail);
    } else if (hasSuffix(typeName, 'Detail_Loop')) {
      body = describeLoop(detail);
    } else {
      body = describeUnknownDetail(detail);
      known = false;
    }
    return {
      key: position + '|' + typeName,
      title: detailShortName(typeName) || 'Detail',
      typeName: typeName,
      known: known,
      fields: body.fields,
      groups: body.groups
    };
  }

  // The path key is the ancestry, so the readable trail comes straight out of
  // it. Escaped separators are put back, since these are names, not keys.
  function pathNames(path) {
    if (!path || path === ROOT_PATH) return [];
    return path.split('/').map(function (part) {
      return part.replace(/%2F/g, '/').replace(/%25/g, '%');
    });
  }

  // Cumulative path keys with their readable names, so each crumb can select
  // the ancestor it names. Splitting on "/" is safe: PathKey escapes a name's
  // own separator as %2F.
  function pathCrumbs(path) {
    if (!path || path === ROOT_PATH) return [];
    var parts = path.split('/');
    var out = [];
    var acc = '';
    for (var i = 0; i < parts.length; i++) {
      acc = acc ? acc + '/' + parts[i] : parts[i];
      out.push({ key: acc, name: parts[i].replace(/%2F/g, '/').replace(/%25/g, '%') });
    }
    return out;
  }

  function describeDock(index, path) {
    var entry = index.get(path);
    if (!entry || path === ROOT_PATH) return null;
    var node = entry.node;
    var details = node.details || [];
    return {
      path: path,
      name: node.name || '',
      type: node.type || '',
      status: effectiveStatus(node),
      duration: formatDuration(node.duration),
      serverId: node.serverId || '',
      failFast: Boolean(node.failFastTriggered),
      trail: pathNames(path),
      crumbs: pathCrumbs(path),
      childCount: entry.childPaths.length,
      unhealthyDescendants: countUnhealthyDescendants(index, path),
      messages: node.messages || [],
      details: details.map(describeDetail)
    };
  }

  // Built from the model, never from rendered rows: the unhealthy descendants
  // of a collapsed container have no row to scrape.
  function collectUnhealthy(index) {
    var out = [];
    index.forEach(function (entry, path) {
      if (path === ROOT_PATH || !isUnhealthy(effectiveStatus(entry.node))) return;
      out.push({
        path: path,
        name: entry.node.name || '',
        type: entry.node.type || '',
        status: effectiveStatus(entry.node),
        trail: pathNames(path)
      });
    });
    out.sort(function (a, b) { return a.path < b.path ? -1 : (a.path > b.path ? 1 : 0); });
    return out;
  }

  function collectFailFast(index) {
    var out = [];
    index.forEach(function (entry, path) {
      if (path === ROOT_PATH || !entry.node.failFastTriggered) return;
      out.push({ path: path, name: entry.node.name || '', trail: pathNames(path) });
    });
    out.sort(function (a, b) { return a.path < b.path ? -1 : (a.path > b.path ? 1 : 0); });
    return out;
  }

  function formatAge(ms) {
    if (!Number.isFinite(ms) || ms < 0) return '';
    if (ms < 1000) return 'just now';
    var seconds = Math.floor(ms / 1000);
    if (seconds < 60) return seconds + 's ago';
    var minutes = Math.floor(seconds / 60);
    if (minutes < 60) return minutes + 'm ' + (seconds % 60) + 's ago';
    var hours = Math.floor(minutes / 60);
    return hours + 'h ' + (minutes % 60) + 'm ago';
  }

  // Whole seconds since a scan started, null until the server has said when
  // that was. Zero is a real answer and must not read as "unknown".
  function elapsedSeconds(startedAt) {
    if (!startedAt) return null;
    var ms = Date.now() - startedAt.getTime();
    return ms < 0 ? 0 : Math.floor(ms / 1000);
  }

  function formatInterval(ms) {
    if (!Number.isFinite(ms) || ms <= 0) return '';
    var seconds = Math.round(ms / 1000);
    if (seconds < 60) return seconds + 's';
    var minutes = Math.floor(seconds / 60);
    var remainder = seconds % 60;
    return remainder ? minutes + 'm ' + remainder + 's' : minutes + 'm';
  }

  // Connection severity comes from the server, which already maps IDLE to
  // benign: with manual refresh the channel idles after thirty minutes, and
  // an alarm for that is a false alarm.
  var CONNECTION_BANNERS = {
    error: {
      severity: 'error',
      title: 'Cannot reach the health server.',
      body: 'The gRPC channel is in TRANSIENT_FAILURE and is retrying.',
      staleBody: ' Anything below is the last successful scan, not the current state.'
    },
    terminal: {
      severity: 'error',
      title: 'The gRPC channel has shut down.',
      body: 'It will not reconnect. Restart ph ui to resume scanning.'
    }
  };

  // Every degraded state the dashboard can be in, as data. Ordered worst
  // first, and fail-fast leads because a partial estate shown as complete is
  // the worst thing this UI can do.
  function computeBanners(state) {
    var out = [];
    var connection = state.connection;
    var failFast = collectFailFast(state.index);

    if (failFast.length > 0) {
      out.push({
        key: 'fail-fast',
        severity: 'error',
        title: 'Results are incomplete: fail-fast stopped a scan.',
        body: failFast.length === 1
          ? 'Checks after ' + failFast[0].name + ' were never run, so components missing from this tree are unknown, not healthy.'
          : failFast.length + ' components triggered fail-fast. Checks after them were never run, so components missing from this tree are unknown, not healthy.',
        entries: failFast
      });
    }

    if (state.streamStopped) {
      out.push({
        key: 'stream-stopped',
        severity: 'error',
        title: 'The dashboard server stopped.',
        body: 'The event stream was closed deliberately and will not reconnect. Restart ph ui and reload this page.'
      });
    } else if (state.streamState === 'closed') {
      out.push({
        key: 'stream-closed',
        severity: 'error',
        title: 'The event stream closed.',
        body: 'The browser will not retry on its own. Reload this page.'
      });
    } else if (state.streamState === 'reconnecting') {
      out.push({
        key: 'stream-reconnecting',
        severity: 'warn',
        title: 'Lost the event stream.',
        // A stream can drop before any snapshot lands, or mid-frame after the
        // scan event, so the body must not promise content that is not there.
        body: state.haveSnapshot
          ? 'Reconnecting. Anything below is the last snapshot received.'
          : 'Reconnecting. No snapshot arrived before the stream dropped, so there is nothing below yet.'
      });
    }

    if (connection && CONNECTION_BANNERS[connection.severity]) {
      var spec = CONNECTION_BANNERS[connection.severity];
      // The stale-tree sentence only holds once something has been drawn.
      var stale = (spec.staleBody && state.haveSnapshot) ? spec.staleBody : '';
      out.push({
        key: 'connection-' + connection.severity,
        severity: spec.severity,
        title: spec.title,
        body: spec.body + stale + ' Target: ' + (connection.target || 'unknown') + ' (' + connection.state + ').'
      });
    }

    // The press itself failing is not the scan failing: nothing was ever
    // asked for, so it needs its own line rather than being folded into one
    // about the estate.
    if (state.triggerError) {
      out.push({
        key: 'trigger-error',
        severity: 'error',
        title: 'Scan now could not reach the dashboard server.',
        body: 'The request failed: ' + state.triggerError +
          '. Nothing was scanned, and the tree below is unchanged. Check that ph ui is still running, then reload this page.'
      });
    }

    if (state.lastError) {
      var first = !state.haveSnapshot;
      out.push({
        key: 'scan-error',
        severity: first ? 'error' : 'warn',
        title: first ? 'The first scan failed.' : 'The last scan failed.',
        body: 'Dialled ' + (connection ? connection.target : 'the server') + '. ' +
          state.lastError.code + ': ' + state.lastError.error +
          (first ? '' : ' The tree below is the last successful scan.')
      });
    }

    if (!state.haveSnapshot && !state.lastError) {
      var scanning = state.scanState === 'scanning';
      var elapsed = elapsedSeconds(state.scanStartedAt);
      out.push({
        key: 'no-snapshot',
        severity: 'info',
        // A first scan of a real estate takes seconds, and this banner is the
        // largest thing on a cold screen, so it carries the count too rather
        // than leaving it to the dim liveness line alone.
        title: scanning
          ? (elapsed === null ? 'Scanning.' : 'Scanning, ' + elapsed + 's elapsed.')
          : 'No snapshot yet.',
        body: 'Waiting for the first scan of ' + (connection ? connection.target : 'the health server') + '.'
      });
    }

    if (state.haveSnapshot && state.componentCount === 0) {
      out.push({
        key: 'empty-estate',
        severity: 'warn',
        title: 'Connected, but the server reported no components.',
        body: 'The scan of ' + (connection ? connection.target : 'the health server') +
          ' succeeded and returned an empty tree. Check the server config.'
      });
    }

    if (!state.haveSnapshot && connection && connection.severity === 'transient') {
      out.push({
        key: 'connection-transient',
        severity: 'info',
        title: 'Connecting.',
        body: 'The gRPC channel is dialling ' + (connection.target || 'the health server') + '.'
      });
    }

    return out;
  }

  // ---------------------------------------------------------------------
  // Graph layout: Reingold-Tilford in Buchheim's linear-time form, laid out
  // left to right. Depth advances on x, siblings stack on y. The estate is
  // shallow and very wide, six columns deep with one fan-out of 43, so the
  // top-down transpose would be a canvas no viewport could read. Pure: an
  // index, a visibility map and a text measurer in, geometry out.
  // ---------------------------------------------------------------------

  var SVG_NS = 'http://www.w3.org/2000/svg';
  var GRAPH_ROW_PITCH = 24;
  var GRAPH_NODE_HEIGHT = 18;
  var GRAPH_COLUMN_GAP = 44;
  var GRAPH_MIN_COLUMN = 108;
  var GRAPH_PAD = 9;
  var GRAPH_DOT_R = 4;
  var GRAPH_GAP = 6;
  var GRAPH_BADGE_PAD = 6;
  var GRAPH_FONT_STACK = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif';
  var GRAPH_NAME_FONT = '500 11px ' + GRAPH_FONT_STACK;
  var GRAPH_TYPE_FONT = '500 9px ' + GRAPH_FONT_STACK;
  // Below this scale an 11px name renders under 8.25px on screen, so ordinary
  // labels are dropped. Selected and unhealthy names stay, counter-scaled to
  // hold that same 8.25px, because an alert nobody can name is not an alert.
  var GRAPH_LABEL_MIN_K = 0.75;
  // Duration text is gated by a floor that falls as you zoom in, so a hidden
  // hop is one zoom away rather than gone. Largest floor first.
  var GRAPH_DURATION_STEPS = [0.1, 0.01];
  var GRAPH_DURATION_ZOOMS = [1.25, 2];
  var GRAPH_MIN_K = 0.25;
  var GRAPH_MAX_K = 3;
  var GRAPH_FIT_PAD = 24;
  var GRAPH_CLICK_SLOP = 3;
  // Second press on the same node inside this window expands or collapses it,
  // the graph's answer to the rail's toggle.
  var GRAPH_DOUBLE_CLICK_MS = 400;
  var GRAPH_FOLLOW_KEY = 'ph-ui-graph-follow';
  // Follow centres a selection, and lifts the zoom only when it is below this,
  // so it never pulls back from a level the viewer chose.
  var GRAPH_FOLLOW_MIN_K = 1;
  var GRAPH_FOLLOW_MS = 180;

  // The hidden property is HTMLElement only, so assigning it on an SVG node
  // sets a dead JS property and never the attribute the stylesheet matches.
  function setHidden(el, hidden) {
    if (!el) return;
    if (hidden) el.setAttribute('hidden', '');
    else el.removeAttribute('hidden');
  }

  function clamp(value, min, max) {
    return Math.min(Math.max(value, min), max);
  }

  // 0 shows every duration, higher numbers hide progressively faster hops.
  function graphDurationFloor(k) {
    for (var i = GRAPH_DURATION_ZOOMS.length - 1; i >= 0; i--) {
      if (k >= GRAPH_DURATION_ZOOMS[i]) return GRAPH_DURATION_ZOOMS.length - 1 - i;
    }
    return GRAPH_DURATION_STEPS.length;
  }

  // The rung a hop clears, so a label survives when it is at or above the floor.
  function durationMagnitude(seconds) {
    if (seconds === null || !(seconds > 0)) return null;
    for (var i = 0; i < GRAPH_DURATION_STEPS.length; i++) {
      if (seconds >= GRAPH_DURATION_STEPS[i]) return GRAPH_DURATION_STEPS.length - i;
    }
    return 0;
  }

  // The path key already encodes the ancestry, so the chain from the root to
  // a selection is a prefix walk over it and never a DOM walk or a stored
  // parent pointer that could drift from the model. Names escape their own
  // slashes, so splitting on "/" is exact.
  function selectionChain(path) {
    var chain = new Set();
    if (!path) return chain;
    chain.add(ROOT_PATH);
    if (path === ROOT_PATH) return chain;
    var parts = path.split('/');
    var accumulated = '';
    for (var i = 0; i < parts.length; i++) {
      accumulated = i === 0 ? parts[0] : accumulated + '/' + parts[i];
      chain.add(accumulated);
    }
    return chain;
  }

  // Canvas measurement, not getComputedTextLength: widths are needed before
  // anything is in the document, and the cache makes a re-layout free.
  var measureContext = null;
  var measureCache = new Map();

  function measureGraphText(text, font) {
    var key = font + '|' + text;
    var cached = measureCache.get(key);
    if (cached !== undefined) return cached;
    if (!measureContext) measureContext = document.createElement('canvas').getContext('2d');
    measureContext.font = font;
    var width = measureContext.measureText(text).width;
    measureCache.set(key, width);
    return width;
  }

  // Only visible nodes enter the layout: a collapsed parent is one badged
  // node, so the geometry never places more than the view is showing.
  function buildGraphTree(index, visibility) {
    var rootEntry = index.get(ROOT_PATH);
    if (!rootEntry) return null;

    function make(entry, depth, parent) {
      var vis = visibility.get(entry.path);
      var open = entry.path === ROOT_PATH ? true : Boolean(vis && vis.open);
      var node = {
        path: entry.path, entry: entry, depth: depth, parent: parent, i: 0,
        children: [], collapsed: entry.childPaths.length > 0 && !open,
        prelim: 0, mod: 0, shift: 0, change: 0, thread: null, ancestor: null, y: 0
      };
      node.ancestor = node;
      if (!open) return node;
      for (var c = 0; c < entry.childPaths.length; c++) {
        var childEntry = index.get(entry.childPaths[c]);
        var childVis = visibility.get(childEntry.path);
        if (childVis && childVis.hidden) continue;
        var child = make(childEntry, depth + 1, node);
        child.i = node.children.length;
        node.children.push(child);
      }
      return node;
    }
    return make(rootEntry, 0, null);
  }

  // Measures one node's box and the x of everything inside it. Width is fixed
  // by content and never by zoom, so dropping labels never moves anything.
  function describeGraphNode(node, index, opts) {
    var raw = node.entry.node;
    var isRoot = node.path === ROOT_PATH;
    var name = isRoot ? opts.rootLabel : (raw.name || '');
    var type = isRoot ? '' : (raw.type || '');
    var typeText = type ? displayType(type).toUpperCase() : '';
    var unhealthy = node.collapsed ? countUnhealthyDescendants(index, node.path) : 0;
    var badgeText = node.collapsed
      ? String(node.entry.childPaths.length) + (unhealthy > 0 ? ' · ' + unhealthy : '')
      : '';

    var nameWidth = name ? Math.ceil(opts.measure(name, GRAPH_NAME_FONT)) : 0;
    var typeWidth = typeText ? Math.ceil(opts.measure(typeText, GRAPH_TYPE_FONT)) : 0;
    var badgeWidth = badgeText ? Math.ceil(opts.measure(badgeText, GRAPH_TYPE_FONT)) + 2 * GRAPH_BADGE_PAD : 0;

    node.name = name;
    node.type = type;
    node.typeText = typeText;
    node.badgeText = badgeText;
    node.badgeWidth = badgeWidth;
    node.childCount = node.entry.childPaths.length;
    node.unhealthyDescendants = unhealthy;
    node.status = effectiveStatus(raw);
    node.satellite = Boolean(raw.serverId);

    // A component's duration already covers its own subtree, so this is the
    // cost of the hop into it and belongs to the edge above, not to the box.
    node.durationSeconds = parseDurationSeconds(raw.duration);
    node.durationText = formatDuration(raw.duration);
    node.slowestSibling = false;

    node.dotCx = GRAPH_PAD + GRAPH_DOT_R;
    node.nameX = node.dotCx + GRAPH_DOT_R + GRAPH_GAP;
    node.typeX = node.nameX + nameWidth + (nameWidth ? GRAPH_GAP : 0);
    node.badgeX = node.typeX + typeWidth + (typeWidth ? GRAPH_GAP : 0);
    node.width = node.badgeX + badgeWidth + GRAPH_PAD;
  }

  // Among one parent's visible children the largest duration is the hop that
  // contributed most, and marking it under every parent traces a critical
  // path from the root. A tie keeps the first child, so the mark never
  // flickers between two equal siblings across a push.
  function markSlowestSiblings(walked) {
    for (var i = 0; i < walked.length; i++) {
      var children = walked[i].children;
      var slowest = null;
      for (var c = 0; c < children.length; c++) {
        var seconds = children[c].durationSeconds;
        if (seconds === null) continue;
        if (slowest === null || seconds > slowest.durationSeconds) slowest = children[c];
      }
      if (slowest) slowest.slowestSibling = true;
    }
  }

  function graphNextLeft(v) {
    return v.children.length ? v.children[0] : v.thread;
  }

  function graphNextRight(v) {
    return v.children.length ? v.children[v.children.length - 1] : v.thread;
  }

  function graphMoveSubtree(wm, wp, shift) {
    var subtrees = wp.i - wm.i;
    wp.change -= shift / subtrees;
    wp.shift += shift;
    wm.change += shift / subtrees;
    wp.prelim += shift;
    wp.mod += shift;
  }

  function graphExecuteShifts(v) {
    var shift = 0;
    var change = 0;
    for (var i = v.children.length - 1; i >= 0; i--) {
      var w = v.children[i];
      w.prelim += shift;
      w.mod += shift;
      change += w.change;
      shift += w.shift + change;
    }
  }

  function graphAncestor(vim, v, defaultAncestor) {
    return vim.ancestor && vim.ancestor.parent === v.parent ? vim.ancestor : defaultAncestor;
  }

  // Walks the two subtree contours in step and pushes v down by whatever the
  // left contour of the trees already placed demands. The threads are what
  // keep that walk linear instead of rescanning every level.
  function graphApportion(v, defaultAncestor, distance) {
    var w = v.i > 0 ? v.parent.children[v.i - 1] : null;
    if (!w) return defaultAncestor;

    var vip = v;
    var vop = v;
    var vim = w;
    var vom = vip.parent.children[0];
    var sip = vip.mod;
    var sop = vop.mod;
    var sim = vim.mod;
    var som = vom.mod;

    while (graphNextRight(vim) && graphNextLeft(vip)) {
      vim = graphNextRight(vim);
      vip = graphNextLeft(vip);
      vom = graphNextLeft(vom);
      vop = graphNextRight(vop);
      vop.ancestor = v;
      var shift = (vim.prelim + sim) - (vip.prelim + sip) + distance;
      if (shift > 0) {
        graphMoveSubtree(graphAncestor(vim, v, defaultAncestor), v, shift);
        sip += shift;
        sop += shift;
      }
      sim += vim.mod;
      sip += vip.mod;
      som += vom.mod;
      sop += vop.mod;
    }
    if (graphNextRight(vim) && !graphNextRight(vop)) {
      vop.thread = graphNextRight(vim);
      vop.mod += sim - sop;
    }
    if (graphNextLeft(vip) && !graphNextLeft(vom)) {
      vom.thread = graphNextLeft(vip);
      vom.mod += sip - som;
      return v;
    }
    return defaultAncestor;
  }

  function graphFirstWalk(v, distance) {
    if (!v.children.length) {
      v.prelim = v.i > 0 ? v.parent.children[v.i - 1].prelim + distance : 0;
      return;
    }
    var defaultAncestor = v.children[0];
    for (var i = 0; i < v.children.length; i++) {
      graphFirstWalk(v.children[i], distance);
      defaultAncestor = graphApportion(v.children[i], defaultAncestor, distance);
    }
    graphExecuteShifts(v);
    var midpoint = (v.children[0].prelim + v.children[v.children.length - 1].prelim) / 2;
    if (v.i > 0) {
      v.prelim = v.parent.children[v.i - 1].prelim + distance;
      v.mod = v.prelim - midpoint;
    } else {
      v.prelim = midpoint;
    }
  }

  function graphSecondWalk(v, m, out) {
    v.y = v.prelim + m;
    out.push(v);
    for (var i = 0; i < v.children.length; i++) {
      graphSecondWalk(v.children[i], m + v.mod, out);
    }
  }

  function layoutGraph(index, visibility, opts) {
    var root = buildGraphTree(index, visibility);
    if (!root) return { nodes: [], edges: [], width: 0, height: 0 };

    var walked = [];
    graphFirstWalk(root, GRAPH_ROW_PITCH);
    graphSecondWalk(root, 0, walked);

    var columns = [];
    var minY = Infinity;
    var maxY = -Infinity;
    for (var i = 0; i < walked.length; i++) {
      var node = walked[i];
      describeGraphNode(node, index, opts);
      columns[node.depth] = Math.max(columns[node.depth] || GRAPH_MIN_COLUMN, node.width);
      if (node.y < minY) minY = node.y;
      if (node.y > maxY) maxY = node.y;
    }
    markSlowestSiblings(walked);

    var columnX = [];
    var offset = 0;
    for (var d = 0; d < columns.length; d++) {
      columnX[d] = offset;
      offset += columns[d] + GRAPH_COLUMN_GAP;
    }

    var nodes = [];
    var edges = [];
    for (var n = 0; n < walked.length; n++) {
      var v = walked[n];
      v.x = columnX[v.depth];
      v.y = v.y - minY + GRAPH_NODE_HEIGHT / 2;
      nodes.push(v);
      if (v.parent) {
        edges.push({ path: v.path, parent: v.parent, child: v });
      }
    }

    return {
      nodes: nodes,
      edges: edges,
      width: columnX[columns.length - 1] + columns[columns.length - 1],
      height: maxY - minY + GRAPH_NODE_HEIGHT
    };
  }

  // ---------------------------------------------------------------------
  // Mutable state. Server data lives in model; view lives separately and is
  // never derived from it, so it survives a push untouched.
  // ---------------------------------------------------------------------

  var model = {
    index: new Map(),
    scanID: null,
    seq: 0,
    observedAt: null,
    transitions: [],
    isFirstSnapshot: true,

    // Liveness and connection health. All of it is server-reported, which is
    // why it lives here and not in view.
    connection: null,
    refreshMs: 0,
    scanState: 'idle',
    queuedFollowUp: false,
    // Server-reported start of the running scan, which is what makes the
    // elapsed readout the scan's own age rather than the age of a click.
    scanStartedAt: null,
    triggerError: null,
    lastScanAt: null,
    lastScanDurationMs: null,
    lastScanChanged: false,
    lastError: null,
    streamState: 'connecting',
    streamStopped: false,
    transitionLog: []
  };

  // focusPath is the roving tab stop: the one rail row carrying tabindex 0,
  // so Tab crosses the whole tree in a single press.
  var view = {
    expanded: new Set(), selected: null, filter: '', typeFilter: null,
    unhealthyOnly: false, zoom: { x: 0, y: 0, k: 1 }, dockOpen: true,
    follow: readStoredFollow(),
    focusPath: null
  };

  var rows = new Map();
  var sawFirstSnapshot = false;
  var es = null;

  // Newest first, capped: the log is a recent-changes list, not an audit
  // trail, and an unbounded one grows for as long as the tab is open.
  var TRANSITION_LOG_LIMIT = 60;

  var graphNodes = new Map();
  var graphEdges = new Map();
  var graphEdgeLabels = new Map();
  var graphPositions = new Map();
  var graphExtent = { width: 0, height: 0 };
  var graphFitted = false;
  var pendingGraphReveal = null;

  var treeRootEl = null;
  var chipsEl = null;
  var railEl = null;
  var railScrollFadeEl = null;
  var graphCanvasEl = null;
  var graphSvgEl = null;
  var graphViewportEl = null;
  var graphEdgeLayerEl = null;
  var graphEdgeLabelLayerEl = null;
  var graphNodeLayerEl = null;
  var graphEmptyEl = null;
  var graphEmptyTextEl = null;
  var railSkeletonEl = null;
  var graphSkeletonEl = null;
  var graphFollowEl = null;
  var graphScrollFadeEl = null;
  var graphZoomReadoutEl = null;
  var targetEl = document.getElementById('target-address');
  var lastScanEl = document.getElementById('last-scan');
  var pillEl = document.getElementById('unhealthy-pill');
  var scanButton = document.getElementById('scan-now');

  var dockEl = null;
  var dock = null;
  var bannersEl = null;
  var syncBanners = null;
  var pillPopover = null;
  var livenessPopover = null;

  // New containers <=25 children default open; bigger ones default
  // collapsed. Existing paths never have this re-applied, which is what
  // lets a manual collapse or expand survive every later push.
  function seedDefaultExpansion(index, addedPaths) {
    for (var i = 0; i < addedPaths.length; i++) {
      var entry = index.get(addedPaths[i]);
      if (entry.childPaths.length > 0 && entry.childPaths.length <= COLLAPSE_THRESHOLD) {
        view.expanded.add(addedPaths[i]);
      }
    }
  }

  // First snapshot only: force every ancestor of an unhealthy node open,
  // even a container the size policy would otherwise collapse.
  function expandAncestorsOfUnhealthy(index) {
    index.forEach(function (entry, path) {
      if (path === ROOT_PATH || !isUnhealthy(effectiveStatus(entry.node))) return;
      var ancestor = entry.parentPath;
      while (ancestor !== null && ancestor !== ROOT_PATH) {
        var ancestorEntry = index.get(ancestor);
        if (!ancestorEntry) break;
        if (ancestorEntry.childPaths.length > 0) view.expanded.add(ancestor);
        ancestor = ancestorEntry.parentPath;
      }
    });
  }

  function destroyRow(path) {
    var row = rows.get(path);
    if (row) {
      row.nodeEl.remove();
      rows.delete(path);
    }
  }

  // ---------------------------------------------------------------------
  // DOM construction and the render pipeline.
  // ---------------------------------------------------------------------

  function buildRailChrome(container) {
    var controls = document.createElement('div');
    controls.className = 'rail-controls';

    var search = document.createElement('input');
    search.type = 'search';
    search.className = 'rail-search';
    search.placeholder = 'Filter components';
    search.setAttribute('aria-label', 'Filter components by name');
    search.addEventListener('input', function () {
      view.filter = search.value.trim();
      render();
    });

    var chips = document.createElement('div');
    chips.className = 'rail-chips';

    controls.appendChild(search);
    controls.appendChild(chips);

    var tree = document.createElement('div');
    tree.className = 'tree-root';
    tree.setAttribute('role', 'tree');

    var fade = document.createElement('div');
    fade.className = 'rail-scroll-fade';

    container.appendChild(controls);
    container.appendChild(buildRailSkeleton());
    container.appendChild(tree);
    container.appendChild(fade);

    return { chips: chips, tree: tree, fade: fade, search: search };
  }

  // Depth and name width of each placeholder row, shaped like a real estate so
  // the wait previews the layout instead of covering it.
  var RAIL_SKELETON_ROWS = [
    [0, 96], [1, 74], [2, 58], [2, 88], [2, 66], [1, 110],
    [0, 82], [1, 92], [2, 70], [2, 54], [1, 64], [0, 100]
  ];

  function buildRailSkeleton() {
    var wrap = document.createElement('div');
    wrap.className = 'rail-skeleton';
    wrap.hidden = true;
    wrap.setAttribute('aria-hidden', 'true');
    for (var i = 0; i < RAIL_SKELETON_ROWS.length; i++) {
      var row = document.createElement('div');
      row.className = 'rail-skeleton-row';
      row.style.paddingLeft = (8 + RAIL_SKELETON_ROWS[i][0] * 16) + 'px';
      var dot = document.createElement('span');
      dot.className = 'rail-skeleton-dot';
      var bar = document.createElement('span');
      bar.className = 'rail-skeleton-bar';
      bar.style.width = RAIL_SKELETON_ROWS[i][1] + 'px';
      row.appendChild(dot);
      row.appendChild(bar);
      wrap.appendChild(row);
    }
    railSkeletonEl = wrap;
    return wrap;
  }

  // Shown only while there is content below the fold, gone once scrolled to
  // the end: a permanent mark would just be decoration, not an affordance.
  function updateRailScrollFade() {
    if (!railEl || !railScrollFadeEl) return;
    var hasMore = railEl.scrollHeight - railEl.scrollTop - railEl.clientHeight > 1;
    railScrollFadeEl.classList.toggle('rail-scroll-fade--visible', hasMore);
  }

  // Zero for a top level component, one per container above it after that.
  function pathDepth(path) {
    var depth = 0;
    var entry = model.index.get(path);
    while (entry && entry.parentPath && entry.parentPath !== ROOT_PATH) {
      depth++;
      entry = model.index.get(entry.parentPath);
    }
    return depth;
  }

  function createRow(path) {
    var nodeEl = document.createElement('div');
    nodeEl.className = 'tree-node';
    nodeEl.dataset.path = path;

    var row = document.createElement('div');
    row.className = 'tree-row';
    // Out of the tab order by default: applyRoving puts exactly one row back
    // in, and every other row is reached with the arrow keys instead.
    row.tabIndex = -1;
    row.dataset.path = path;
    // Fixed for the life of a path, since a path is its own ancestry. CSS
    // turns it into the indent the row's negative margin gives back.
    row.dataset.depth = String(pathDepth(path));
    row.setAttribute('role', 'treeitem');

    var toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.className = 'tree-toggle';
    toggle.tabIndex = -1;

    var dot = document.createElement('span');
    dot.className = 'status-dot';

    var name = document.createElement('span');
    name.className = 'tree-name';

    var typeLabel = document.createElement('button');
    typeLabel.type = 'button';
    typeLabel.className = 'tree-type chip';
    typeLabel.tabIndex = -1;

    var duration = document.createElement('span');
    duration.className = 'tree-duration';

    var badge = document.createElement('span');
    badge.className = 'tree-badge tree-badge--empty';

    // The badge rides with the name, since it describes this one row rather
    // than lining up down the rail.
    var label = document.createElement('span');
    label.className = 'tree-label';
    label.appendChild(name);
    label.appendChild(badge);

    // One right-anchored group, so the two columns after the name stay at
    // the rail's visible edge however wide the widest name makes the row.
    var trailing = document.createElement('div');
    trailing.className = 'tree-trailing';
    trailing.appendChild(typeLabel);
    trailing.appendChild(duration);

    row.appendChild(toggle);
    row.appendChild(dot);
    row.appendChild(label);
    row.appendChild(trailing);

    var children = document.createElement('div');
    children.className = 'tree-children';
    children.setAttribute('role', 'group');

    nodeEl.appendChild(row);
    nodeEl.appendChild(children);

    toggle.addEventListener('click', function (e) {
      e.stopPropagation();
      if (view.expanded.has(path)) view.expanded.delete(path); else view.expanded.add(path);
      render();
    });
    typeLabel.addEventListener('click', function (e) {
      e.stopPropagation();
      // The real type, not the abbreviated label: dataset.type is set from
      // node.type in updateRowContent, textContent may be shortened for display.
      var type = typeLabel.dataset.type;
      if (!type) return;
      view.typeFilter = view.typeFilter === type ? null : type;
      render();
    });
    row.addEventListener('click', function () {
      selectRow(path);
    });

    var bundle = {
      nodeEl: nodeEl, row: row, toggle: toggle, dot: dot, name: name,
      typeLabel: typeLabel, duration: duration, badge: badge,
      trailing: trailing, children: children
    };
    rows.set(path, bundle);
    return bundle;
  }

  // The one entry point for selecting from the rail, so a keyboard Enter or
  // Space does exactly what a click does and cannot drift away from it.
  function selectRow(path) {
    view.selected = path;
    // A ring on a node parked outside the viewport is a ring nobody sees,
    // so the graph pans just far enough to bring it in. Never zooms.
    pendingGraphReveal = path;
    render();
  }

  // ---------------------------------------------------------------------
  // Rail keyboard: one delegated listener for 185 rows, a roving tab stop
  // and the arrow keys role="treeitem" already promises.
  // ---------------------------------------------------------------------

  // Painted order, skipping whatever a filter or a collapsed ancestor has
  // taken out of the flow. Read at keypress time, when layout is clean.
  function visibleRowEls() {
    var all = treeRootEl.querySelectorAll('.tree-row');
    var out = [];
    for (var i = 0; i < all.length; i++) {
      if (all[i].offsetParent !== null) out.push(all[i]);
    }
    return out;
  }

  // Model only, no layout read: a row is in the flow when nothing on its
  // ancestry is filtered out or collapsed shut.
  function rowIsInFlow(path, visibility) {
    var entry = model.index.get(path);
    while (entry && entry.path !== ROOT_PATH) {
      var vis = visibility.get(entry.path);
      if (!vis || vis.hidden) return false;
      var parentPath = entry.parentPath;
      if (parentPath === null || parentPath === ROOT_PATH) return true;
      var parentVis = visibility.get(parentPath);
      if (!parentVis || !parentVis.open) return false;
      entry = model.index.get(parentPath);
    }
    return false;
  }

  function nearestInFlowPath(path, visibility) {
    var entry = path ? model.index.get(path) : null;
    while (entry && entry.path !== ROOT_PATH) {
      if (rowIsInFlow(entry.path, visibility)) return entry.path;
      var parentPath = entry.parentPath;
      entry = (parentPath && parentPath !== ROOT_PATH) ? model.index.get(parentPath) : null;
    }
    return null;
  }

  // Exactly one row in the tab order. Held on view.focusPath so an SSE push,
  // which reconciles rows in place, never moves it or drops it.
  function applyRoving(visibility) {
    var next = nearestInFlowPath(view.focusPath, visibility);
    if (!next) {
      var els = visibleRowEls();
      next = els.length ? els[0].dataset.path : null;
    }
    view.focusPath = next;
    rows.forEach(function (row, path) {
      row.row.tabIndex = path === next ? 0 : -1;
    });
  }

  // Vertical only. The rail scrolls sideways to reach a long name and a
  // focus move must never spend that scroll position.
  function keepRowInView(el) {
    if (!railEl) return;
    var railRect = railEl.getBoundingClientRect();
    var rect = el.getBoundingClientRect();
    if (rect.top < railRect.top) railEl.scrollTop -= railRect.top - rect.top;
    else if (rect.bottom > railRect.bottom) railEl.scrollTop += rect.bottom - railRect.bottom;
  }

  function focusRowEl(el) {
    if (!el) return;
    el.focus({ preventScroll: true });
    keepRowInView(el);
  }

  function onTreeFocusIn(e) {
    var rowEl = e.target.closest ? e.target.closest('.tree-row') : null;
    if (!rowEl || !rowEl.dataset.path || rowEl.dataset.path === view.focusPath) return;
    var previous = rows.get(view.focusPath);
    if (previous) previous.row.tabIndex = -1;
    view.focusPath = rowEl.dataset.path;
    rowEl.tabIndex = 0;
  }

  function onTreeKeydown(e) {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    var rowEl = e.target.closest ? e.target.closest('.tree-row') : null;
    if (!rowEl) return;
    var path = rowEl.dataset.path;
    var entry = model.index.get(path);
    if (!entry) return;

    var key = e.key;
    var isSelect = key === 'Enter' || key === ' ' || key === 'Spacebar';
    // A button inside the row keeps its own activation keys.
    if (isSelect && e.target !== rowEl) return;
    if (isSelect) {
      e.preventDefault();
      selectRow(path);
      return;
    }

    var open = rowEl.getAttribute('aria-expanded') === 'true';
    var hasChildren = entry.childPaths.length > 0;

    if (key === 'ArrowRight') {
      e.preventDefault();
      if (!hasChildren) return;
      if (!open) {
        view.expanded.add(path);
        render();
        return;
      }
    } else if (key === 'ArrowLeft') {
      e.preventDefault();
      if (hasChildren && open) {
        view.expanded.delete(path);
        render();
        return;
      }
      var parentPath = entry.parentPath;
      var parentRow = parentPath && parentPath !== ROOT_PATH ? rows.get(parentPath) : null;
      if (parentRow) focusRowEl(parentRow.row);
      return;
    } else if (key !== 'ArrowDown' && key !== 'ArrowUp' && key !== 'Home' && key !== 'End') {
      return;
    } else {
      e.preventDefault();
    }

    var els = visibleRowEls();
    if (els.length === 0) return;
    if (key === 'Home') { focusRowEl(els[0]); return; }
    if (key === 'End') { focusRowEl(els[els.length - 1]); return; }
    var i = els.indexOf(rowEl);
    if (i < 0) return;
    // ArrowRight on an already open row steps into its first child, which is
    // simply the next painted row.
    var step = key === 'ArrowUp' ? -1 : 1;
    var target = els[i + step];
    if (target) focusRowEl(target);
  }

  function updateRowContent(row, entry, vis, chain) {
    var node = entry.node;
    var status = effectiveStatus(node);

    row.dot.className = 'status-dot status-dot--' + statusClass(status);
    row.name.textContent = node.name || '';
    // Set unconditionally rather than measuring whether the name actually
    // ellipsises: cheap, harmless when it fits, and covers every depth.
    row.name.setAttribute('title', node.name || '');
    row.duration.textContent = formatDuration(node.duration);

    var type = node.type || '';
    row.typeLabel.textContent = displayType(type);
    row.typeLabel.hidden = !type;
    row.typeLabel.dataset.type = type;
    // The full, unabbreviated value: an abbreviation should not cost
    // information, so it is one hover away.
    if (type) {
      row.typeLabel.setAttribute('title', type);
    } else {
      row.typeLabel.removeAttribute('title');
    }
    row.typeLabel.classList.toggle('chip--active', Boolean(type) && view.typeFilter === type);

    row.toggle.textContent = vis.hasChildren ? (vis.open ? '⌄' : '›') : '';
    row.toggle.classList.toggle('tree-toggle--leaf', !vis.hasChildren);
    row.toggle.setAttribute('aria-expanded', String(vis.open));
    // On the treeitem, not just the button: it is what assistive tech reads
    // and what the arrow keys consult for the row's open state.
    if (vis.hasChildren) {
      row.row.setAttribute('aria-expanded', String(vis.open));
    } else {
      row.row.removeAttribute('aria-expanded');
    }

    if (vis.hasChildren && !vis.open) {
      var unhealthy = countUnhealthyDescendants(model.index, entry.path);
      var badgeText = entry.childPaths.length + (unhealthy > 0 ? ' · ' + unhealthy + ' unhealthy' : '');
      row.badge.textContent = badgeText;
      row.badge.setAttribute('title', badgeText);
      row.badge.classList.remove('tree-badge--empty');
      row.badge.classList.toggle('tree-badge--unhealthy', unhealthy > 0);
    } else {
      row.badge.textContent = '';
      row.badge.removeAttribute('title');
      row.badge.classList.add('tree-badge--empty');
      row.badge.classList.remove('tree-badge--unhealthy');
    }

    var selected = view.selected === entry.path;
    row.row.classList.toggle('tree-row--selected', selected);
    // The ancestors of a selection, marked the same way the graph marks its
    // route, so the two surfaces read as one thing. Colour only: a weight
    // change here would widen the rail's max-content box and shift the
    // fixed columns after the name.
    row.row.classList.toggle('tree-row--on-path', !selected && chain.has(entry.path));
    row.row.classList.toggle('tree-row--satellite', Boolean(node.serverId));
    row.row.setAttribute('aria-selected', String(view.selected === entry.path));
  }

  function collectTypes(index) {
    var types = new Set();
    index.forEach(function (entry, path) {
      if (path !== ROOT_PATH && entry.node.type) types.add(entry.node.type);
    });
    return Array.from(types).sort();
  }

  function renderChips() {
    while (chipsEl.firstChild) chipsEl.removeChild(chipsEl.firstChild);

    var unhealthyChip = document.createElement('button');
    unhealthyChip.type = 'button';
    unhealthyChip.className = 'chip chip--unhealthy';
    unhealthyChip.textContent = 'Unhealthy';
    unhealthyChip.classList.toggle('chip--active', view.unhealthyOnly);
    unhealthyChip.addEventListener('click', function () {
      view.unhealthyOnly = !view.unhealthyOnly;
      render();
    });
    chipsEl.appendChild(unhealthyChip);

    var types = collectTypes(model.index);
    for (var i = 0; i < types.length; i++) {
      (function (type) {
        var chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'chip';
        chip.textContent = type;
        chip.classList.toggle('chip--active', view.typeFilter === type);
        chip.addEventListener('click', function () {
          view.typeFilter = view.typeFilter === type ? null : type;
          render();
        });
        chipsEl.appendChild(chip);
      })(types[i]);
    }
  }

  // ---------------------------------------------------------------------
  // URL state. The address bar carries what a link has to reproduce: the
  // selection, the filters and the viewport. Preferences the viewer owns
  // (theme, follow, rail width) stay in localStorage, so a shared link never
  // rewrites how someone else's dashboard behaves.
  // ---------------------------------------------------------------------

  var urlSyncTimer = null;
  var pendingUrlPath = null;
  var pendingUrlView = null;

  function readUrlState() {
    var q;
    try {
      q = new URLSearchParams(window.location.search);
    } catch (e) {
      return;
    }
    view.filter = q.get('q') || '';
    view.typeFilter = q.get('t') || null;
    view.unhealthyOnly = q.get('u') === '1';
    // The link wins over the stored preference: someone sharing a followed
    // view means the follow, and the toggle writes storage again on any change.
    var f = q.get('f');
    if (f !== null) view.follow = f === '1';
    pendingUrlPath = q.get('c');
    var v = (q.get('v') || '').split(',');
    if (v.length === 3) {
      var x = parseFloat(v[0]), y = parseFloat(v[1]), k = parseFloat(v[2]);
      if (Number.isFinite(x) && Number.isFinite(y) && Number.isFinite(k)) {
        pendingUrlView = { x: x, y: y, k: clamp(k, GRAPH_MIN_K, GRAPH_MAX_K) };
      }
    }
  }

  // Applied once the first snapshot exists, because a path means nothing
  // before there is an index to resolve it against.
  function applyPendingUrlState() {
    if (pendingUrlView) {
      view.zoom.x = pendingUrlView.x;
      view.zoom.y = pendingUrlView.y;
      view.zoom.k = pendingUrlView.k;
      // A viewport from the link is the viewer's, so the one-shot auto-fit
      // must not overwrite it.
      graphFitted = true;
      pendingUrlView = null;
      applyGraphTransform();
    }
    var path = pendingUrlPath;
    pendingUrlPath = null;
    if (path && model.index.has(path)) selectPath(path);
  }

  function currentUrl() {
    var q = new URLSearchParams();
    if (view.selected) q.set('c', view.selected);
    if (view.filter) q.set('q', view.filter);
    if (view.typeFilter) q.set('t', view.typeFilter);
    if (view.unhealthyOnly) q.set('u', '1');
    if (view.follow) q.set('f', '1');
    var z = view.zoom;
    q.set('v', Math.round(z.x) + ',' + Math.round(z.y) + ',' + z.k.toFixed(3));
    var s = q.toString();
    return window.location.pathname + (s ? '?' + s : '');
  }

  // replaceState, never push: panning would otherwise bury the back button
  // under hundreds of entries.
  function syncUrl() {
    urlSyncTimer = null;
    try {
      window.history.replaceState(null, '', currentUrl());
    } catch (e) {
      // Some embeddings forbid history access; the dashboard still works.
    }
  }

  function scheduleUrlSync() {
    if (urlSyncTimer !== null) return;
    urlSyncTimer = window.setTimeout(syncUrl, 250);
  }

  function render() {
    var visibility = computeVisibility(model.index, view);
    var chain = selectionChain(view.selected);
    model.index.forEach(function (entry, path) {
      if (path === ROOT_PATH) return;
      var row = rows.get(path);
      if (!row) {
        row = createRow(path);
        var parentPath = entry.parentPath;
        var parentRow = (parentPath && parentPath !== ROOT_PATH) ? rows.get(parentPath) : null;
        var container = parentRow ? parentRow.children : treeRootEl;
        container.appendChild(row.nodeEl);
      }
      var vis = visibility.get(path);
      updateRowContent(row, entry, vis, chain);
      row.nodeEl.hidden = vis.hidden;
      row.children.hidden = !vis.open;
    });
    applyRoving(visibility);
    renderGraph(visibility);
    renderChips();
    renderPill();
    renderTransitions();
    renderBanners();
    renderDock();
    updateLiveness();
    scheduleUrlSync();
    updateScanButton();
    updateRailScrollFade();
    // Last, once the dock and the banners have taken their height: reading
    // the canvas box here is a read after every write that can change it.
    takePendingGraphReveal();
  }

  // ---------------------------------------------------------------------
  // Rail resize: drag, double-click reset and arrow-key adjustment on the
  // separator between the rail and the canvas. Width lives in the
  // --rail-width custom property, the same one theme.js restores pre-paint,
  // so this never has to reposition anything by hand.
  // ---------------------------------------------------------------------

  // These four constants and clampRailWidth are duplicated in theme.js, which
  // restores the width pre-paint and cannot import from here. Keep both in sync.
  var RAIL_WIDTH_KEY = 'ph-ui-rail-width';
  var RAIL_WIDTH_DEFAULT = 280;
  var RAIL_WIDTH_MIN = 200;
  var RAIL_WIDTH_MAX_RATIO = 0.45;

  function clampRailWidth(width) {
    var max = window.innerWidth * RAIL_WIDTH_MAX_RATIO;
    return Math.min(Math.max(width, RAIL_WIDTH_MIN), max);
  }

  function readStoredFollow() {
    try {
      return window.localStorage.getItem(GRAPH_FOLLOW_KEY) === '1';
    } catch (e) {
      return false;
    }
  }

  function writeStoredFollow(on) {
    try {
      if (on) window.localStorage.setItem(GRAPH_FOLLOW_KEY, '1');
      else window.localStorage.removeItem(GRAPH_FOLLOW_KEY);
    } catch (e) {
      // Blocked site data: the toggle just will not survive a reload.
    }
  }

  function writeStoredRailWidth(width) {
    try {
      window.localStorage.setItem(RAIL_WIDTH_KEY, String(Math.round(width)));
    } catch (e) {
      // Blocked site data: same fallback as theme.js, the width just won't
      // survive a reload.
    }
  }

  function currentRailWidth(root) {
    var raw = getComputedStyle(root).getPropertyValue('--rail-width');
    var value = parseFloat(raw);
    return Number.isFinite(value) ? value : RAIL_WIDTH_DEFAULT;
  }

  function isRailCollapsed(root) {
    return root.getAttribute('data-rail-collapsed') === 'true';
  }

  function wireRailResize(handle, root) {
    var dragging = false;
    var startX = 0;
    var startWidth = 0;

    function updateAria(width) {
      handle.setAttribute('aria-valuenow', String(Math.round(width)));
      handle.setAttribute('aria-valuemin', String(RAIL_WIDTH_MIN));
      handle.setAttribute('aria-valuemax', String(Math.round(window.innerWidth * RAIL_WIDTH_MAX_RATIO)));
    }

    function applyWidth(width) {
      var clamped = clampRailWidth(width);
      root.style.setProperty('--rail-width', clamped + 'px');
      updateAria(clamped);
      return clamped;
    }

    function onMove(e) {
      if (!dragging) return;
      applyWidth(startWidth + (e.clientX - startX));
    }

    // Pointer events, not mouse events: setPointerCapture guarantees one of
    // pointerup, pointercancel or lostpointercapture reaches the handle even
    // if the button is released outside the window, so the drag can never be
    // abandoned with data-rail-dragging stuck on. Idempotent since more than
    // one of those events can fire for a single drag.
    function endDrag() {
      if (!dragging) return;
      dragging = false;
      root.removeAttribute('data-rail-dragging');
      writeStoredRailWidth(currentRailWidth(root));
      handle.removeEventListener('pointermove', onMove);
      handle.removeEventListener('pointerup', endDrag);
      handle.removeEventListener('pointercancel', endDrag);
      handle.removeEventListener('lostpointercapture', endDrag);
    }

    handle.addEventListener('pointerdown', function (e) {
      if (isRailCollapsed(root)) return;
      // Suppresses text selection, not focus: focus is restored explicitly
      // below so arrow key, Home and End adjustment work right after a drag.
      e.preventDefault();
      handle.focus();
      dragging = true;
      startX = e.clientX;
      startWidth = currentRailWidth(root);
      root.setAttribute('data-rail-dragging', 'true');
      handle.setPointerCapture(e.pointerId);
      handle.addEventListener('pointermove', onMove);
      handle.addEventListener('pointerup', endDrag);
      handle.addEventListener('pointercancel', endDrag);
      handle.addEventListener('lostpointercapture', endDrag);
    });

    handle.addEventListener('dblclick', function () {
      if (isRailCollapsed(root)) return;
      writeStoredRailWidth(applyWidth(RAIL_WIDTH_DEFAULT));
    });

    handle.addEventListener('keydown', function (e) {
      if (isRailCollapsed(root)) return;
      var step = 16;
      var width = currentRailWidth(root);
      if (e.key === 'ArrowLeft') {
        width = applyWidth(width - step);
      } else if (e.key === 'ArrowRight') {
        width = applyWidth(width + step);
      } else if (e.key === 'Home') {
        width = applyWidth(RAIL_WIDTH_MIN);
      } else if (e.key === 'End') {
        width = applyWidth(window.innerWidth * RAIL_WIDTH_MAX_RATIO);
      } else {
        return;
      }
      e.preventDefault();
      writeStoredRailWidth(width);
    });

    updateAria(currentRailWidth(root));

    // Reclamps the live width against a new window.innerWidth without
    // touching storage, called from init's window resize listener.
    function reclampToViewport() {
      applyWidth(currentRailWidth(root));
    }

    return { reclampToViewport: reclampToViewport };
  }

  // ---------------------------------------------------------------------
  // Graph rendering: one keyed element per path, reconciled in place exactly
  // as the rail's rows are, so a push never disturbs pan, zoom or selection.
  // Identity is read from dataset.path, never from rendered text.
  // ---------------------------------------------------------------------

  function svgEl(name, className) {
    var el = document.createElementNS(SVG_NS, name);
    if (className) el.setAttribute('class', className);
    return el;
  }

  function buildGraphChrome(container) {
    var svg = svgEl('svg', 'graph-svg');
    svg.setAttribute('tabindex', '0');
    svg.setAttribute('role', 'application');
    svg.setAttribute('aria-label', 'Component topology');

    var viewport = svgEl('g', 'graph-viewport');
    var edgeLayer = svgEl('g', 'graph-edges');
    // Its own layer between the two: above every edge so no other line paints
    // over a label, below every node box so the boxes stay unobstructed.
    var edgeLabelLayer = svgEl('g', 'graph-edge-labels');
    var nodeLayer = svgEl('g', 'graph-nodes');
    viewport.appendChild(edgeLayer);
    viewport.appendChild(edgeLabelLayer);
    viewport.appendChild(nodeLayer);
    svg.appendChild(viewport);

    var skeleton = buildGraphSkeleton();
    svg.appendChild(skeleton);

    var empty = document.createElement('p');
    empty.className = 'graph-empty';
    var emptyText = document.createElement('span');
    emptyText.textContent = 'Nothing to draw yet.';
    empty.appendChild(emptyText);

    // Same affordance as the rail's fade, for the same reason: the canvas is
    // deliberately larger than the window and nothing else says so.
    var fade = document.createElement('div');
    fade.className = 'graph-scroll-fade';

    var controls = document.createElement('div');
    controls.className = 'graph-controls';

    function controlButton(label, aria, onClick) {
      var button = document.createElement('button');
      button.type = 'button';
      button.className = 'graph-control';
      button.textContent = label;
      button.setAttribute('aria-label', aria);
      button.setAttribute('title', aria);
      button.addEventListener('click', onClick);
      controls.appendChild(button);
      return button;
    }

    controlButton('-', 'Zoom out', function () { zoomGraphAtCentre(1 / 1.25); });
    // Through the same helper the plus and minus buttons use, so the reset
    // anchors on the viewport centre instead of scaling about the group origin
    // and throwing whatever was being read off screen.
    var readout = controlButton('100%', 'Reset zoom to 100 percent', function () {
      zoomGraphAtCentre(1 / view.zoom.k);
    });
    readout.classList.add('graph-control--readout');
    controlButton('+', 'Zoom in', function () { zoomGraphAtCentre(1.25); });
    controlButton('Fit', 'Fit the graph to the viewport', function () { fitGraph(); });

    var follow = controlButton('◎', 'Follow selection', function () {
      view.follow = !view.follow;
      writeStoredFollow(view.follow);
      syncFollowButton();
      if (view.follow && view.selected) centreGraphNode(view.selected);
    });
    follow.classList.add('graph-control--follow');

    // Built only where the API exists, since a dead control is worse than no
    // control. The handler is attached later, once the graph elements it has
    // to reflow are in place.
    var fullscreen = fullscreenSupported(container)
      ? controlButton(FULLSCREEN_ENTER_ICON, 'Fullscreen', function () {})
      : null;
    if (fullscreen) fullscreen.classList.add('graph-control--fullscreen');

    container.appendChild(svg);
    container.appendChild(empty);
    container.appendChild(fade);
    container.appendChild(controls);

    return {
      svg: svg, viewport: viewport, edges: edgeLayer, edgeLabels: edgeLabelLayer,
      nodes: nodeLayer, empty: empty, emptyText: emptyText, fade: fade,
      readout: readout, fullscreen: fullscreen, follow: follow
    };
  }

  function createGraphNode(path) {
    var g = svgEl('g', 'graph-node');
    g.dataset.path = path;

    var title = svgEl('title', null);
    var box = svgEl('rect', 'graph-node-box');
    box.setAttribute('rx', '5');
    box.setAttribute('y', String(-GRAPH_NODE_HEIGHT / 2));
    box.setAttribute('height', String(GRAPH_NODE_HEIGHT));
    box.setAttribute('x', '0');

    var dot = svgEl('circle', 'graph-node-dot');
    dot.setAttribute('r', String(GRAPH_DOT_R));
    dot.setAttribute('cy', '0');

    var name = svgEl('text', 'graph-node-name');
    name.setAttribute('y', '0');
    name.setAttribute('dy', '0.32em');

    var type = svgEl('text', 'graph-node-type');
    type.setAttribute('y', '0');
    type.setAttribute('dy', '0.32em');

    var badgeBox = svgEl('rect', 'graph-node-badge-box');
    badgeBox.setAttribute('rx', '6');
    badgeBox.setAttribute('y', '-6');
    badgeBox.setAttribute('height', '12');

    var badge = svgEl('text', 'graph-node-badge');
    badge.setAttribute('y', '0');
    badge.setAttribute('dy', '0.32em');

    g.appendChild(title);
    g.appendChild(box);
    g.appendChild(dot);
    g.appendChild(name);
    g.appendChild(type);
    g.appendChild(badgeBox);
    g.appendChild(badge);
    graphNodeLayerEl.appendChild(g);

    var bundle = {
      g: g, title: title, box: box, dot: dot, name: name,
      type: type, badgeBox: badgeBox, badge: badge
    };
    graphNodes.set(path, bundle);
    return bundle;
  }

  function graphNodeTitle(node) {
    var parts = [node.name || '(root)'];
    if (node.type) parts.push(node.type);
    parts.push(node.status.toLowerCase());
    if (node.badgeText) {
      parts.push(node.childCount + ' collapsed' +
        (node.unhealthyDescendants > 0 ? ', ' + node.unhealthyDescendants + ' unhealthy' : ''));
    }
    // The badge advertises hidden children, so the surface showing it has to
    // say how to reach them.
    if (node.childCount > 0) {
      parts.push(node.collapsed ? 'double click to expand' : 'double click to collapse');
    }
    return parts.join(' · ');
  }

  function updateGraphNode(bundle, node, chain) {
    var selected = view.selected === node.path;
    var flagged = selected || isUnhealthy(node.status);
    var onPath = chain.size > 0 && chain.has(node.path);

    bundle.g.setAttribute('transform', 'translate(' + node.x + ',' + node.y + ')');
    bundle.g.setAttribute('class', 'graph-node graph-node--' + statusClass(node.status) +
      (selected ? ' graph-node--selected' : '') +
      (flagged ? ' graph-node--flagged' : '') +
      (node.satellite ? ' graph-node--satellite' : '') +
      (chain.size === 0 ? '' : (onPath ? ' graph-node--on-path' : ' graph-node--off-path')));
    bundle.title.textContent = graphNodeTitle(node);

    bundle.box.setAttribute('width', String(node.width));
    bundle.dot.setAttribute('cx', String(node.dotCx));

    bundle.name.setAttribute('x', String(node.nameX));
    bundle.name.textContent = node.name;

    bundle.type.setAttribute('x', String(node.typeX));
    bundle.type.textContent = node.typeText;

    bundle.badgeBox.setAttribute('x', String(node.badgeX));
    bundle.badgeBox.setAttribute('width', String(node.badgeWidth));
    bundle.badgeBox.setAttribute('visibility', node.badgeText ? 'visible' : 'hidden');
    bundle.badge.setAttribute('x', String(node.badgeX + node.badgeWidth / 2));
    bundle.badge.textContent = node.badgeText;
    bundle.badge.setAttribute('class', 'graph-node-badge' +
      (node.unhealthyDescendants > 0 ? ' graph-node-badge--unhealthy' : ''));
  }

  function createGraphEdge(path) {
    var el = svgEl('path', 'graph-edge');
    el.dataset.path = path;
    graphEdgeLayerEl.appendChild(el);
    graphEdges.set(path, el);
    return el;
  }

  function updateGraphEdge(el, edge, chain) {
    var x1 = edge.parent.x + edge.parent.width;
    var y1 = edge.parent.y;
    var x2 = edge.child.x;
    var y2 = edge.child.y;
    var bend = Math.max((x2 - x1) / 2, 8);
    el.setAttribute('d', 'M' + x1 + ' ' + y1 +
      ' C' + (x1 + bend) + ' ' + y1 + ' ' + (x2 - bend) + ' ' + y2 + ' ' + x2 + ' ' + y2);
    // Keyed by the child path, which is also how the chain names an edge: an
    // edge is on the route exactly when its child is.
    var onPath = chain.size > 0 && chain.has(edge.child.path);
    el.setAttribute('class', 'graph-edge' +
      (isUnhealthy(edge.child.status) ? ' graph-edge--unhealthy' : '') +
      (chain.size === 0 ? '' : (onPath ? ' graph-edge--on-path' : ' graph-edge--off-path')));
  }

  // The label rides the child end, where the curve has flattened out, and the
  // marker sits on the line just short of the box. Both stay inside the
  // column gap, which is why neither can ever land on a node box.
  var GRAPH_DURATION_INSET = 10;
  var GRAPH_DURATION_RISE = 5;
  var GRAPH_MARKER_INSET = 3.5;
  var GRAPH_MARKER_R = 2.5;

  function createGraphEdgeLabel(path) {
    var g = svgEl('g', 'graph-edge-label');
    g.dataset.path = path;

    var marker = svgEl('circle', 'graph-edge-marker');
    marker.setAttribute('r', String(GRAPH_MARKER_R));

    var text = svgEl('text', 'graph-edge-duration');
    text.setAttribute('dy', '0.32em');

    g.appendChild(marker);
    g.appendChild(text);
    graphEdgeLabelLayerEl.appendChild(g);

    var bundle = { g: g, marker: marker, text: text };
    graphEdgeLabels.set(path, bundle);
    return bundle;
  }

  function updateGraphEdgeLabel(bundle, edge, chain) {
    var child = edge.child;
    var onPath = chain.size > 0 && chain.has(child.path);
    var slowest = Boolean(child.slowestSibling) && Boolean(child.durationText);

    var magnitude = durationMagnitude(child.durationSeconds);

    bundle.g.setAttribute('class', 'graph-edge-label' +
      (slowest ? ' graph-edge-label--slowest' : '') +
      (isUnhealthy(child.status) ? ' graph-edge-label--unhealthy' : '') +
      (chain.size === 0 ? '' : (onPath ? ' graph-edge-label--on-path' : ' graph-edge-label--off-path')));
    if (magnitude === null) bundle.g.removeAttribute('data-magnitude');
    else bundle.g.setAttribute('data-magnitude', String(magnitude));

    bundle.text.setAttribute('x', String(child.x - GRAPH_DURATION_INSET));
    bundle.text.setAttribute('y', String(child.y - GRAPH_DURATION_RISE));
    bundle.text.textContent = child.durationText;

    bundle.marker.setAttribute('cx', String(child.x - GRAPH_MARKER_INSET));
    bundle.marker.setAttribute('cy', String(child.y));
    bundle.marker.setAttribute('visibility', slowest ? 'visible' : 'hidden');
  }

  function pruneGraph(store, seen, remove) {
    var stale = [];
    store.forEach(function (value, key) {
      if (!seen.has(key)) stale.push(key);
    });
    for (var i = 0; i < stale.length; i++) {
      remove(store.get(stale[i]));
      store.delete(stale[i]);
    }
  }

  // A small tree in the same shapes the real one uses, so the wait previews
  // the layout. Coordinates are a fixed sketch, never derived from data.
  var GRAPH_SKELETON_NODES = [
    [30, 150], [190, 70], [190, 150], [190, 230],
    [350, 40], [350, 100], [350, 190], [350, 260], [350, 320]
  ];
  var GRAPH_SKELETON_EDGES = [
    [30, 150, 190, 70], [30, 150, 190, 150], [30, 150, 190, 230],
    [190, 70, 350, 40], [190, 70, 350, 100],
    [190, 150, 350, 190], [190, 230, 350, 260], [190, 230, 350, 320]
  ];

  function buildGraphSkeleton() {
    var g = svgEl('g', 'graph-skeleton');
    g.setAttribute('aria-hidden', 'true');
    for (var e = 0; e < GRAPH_SKELETON_EDGES.length; e++) {
      var c = GRAPH_SKELETON_EDGES[e];
      var bend = (c[2] - c[0]) / 2;
      var path = svgEl('path', 'graph-skeleton-edge');
      path.setAttribute('d', 'M' + c[0] + ' ' + c[1] +
        ' C' + (c[0] + bend) + ' ' + c[1] + ' ' + (c[2] - bend) + ' ' + c[3] + ' ' + c[2] + ' ' + c[3]);
      g.appendChild(path);
    }
    for (var n = 0; n < GRAPH_SKELETON_NODES.length; n++) {
      var p = GRAPH_SKELETON_NODES[n];
      var rect = svgEl('rect', 'graph-skeleton-node');
      rect.setAttribute('x', String(p[0] - 13));
      rect.setAttribute('y', String(p[1] - 9));
      rect.setAttribute('width', '26');
      rect.setAttribute('height', '18');
      rect.setAttribute('rx', '5');
      g.appendChild(rect);
    }
    graphSkeletonEl = g;
    return g;
  }

  function renderGraph(visibility) {
    if (!graphSvgEl) return;

    // Same source as the header, not a second derivation of it: this
    // dashboard is a client of exactly one server, so the root is that
    // server, spelled exactly as the header spells it.
    var layout = layoutGraph(model.index, visibility, {
      measure: measureGraphText,
      rootLabel: targetEl.textContent
    });
    graphExtent = { width: layout.width, height: layout.height };

    var chain = selectionChain(view.selected);
    var seenNodes = new Set();
    graphPositions.clear();
    for (var i = 0; i < layout.nodes.length; i++) {
      var node = layout.nodes[i];
      var bundle = graphNodes.get(node.path) || createGraphNode(node.path);
      updateGraphNode(bundle, node, chain);
      graphPositions.set(node.path, node);
      seenNodes.add(node.path);
    }
    pruneGraph(graphNodes, seenNodes, function (bundle) { bundle.g.remove(); });

    var seenEdges = new Set();
    for (var e = 0; e < layout.edges.length; e++) {
      var edge = layout.edges[e];
      updateGraphEdge(graphEdges.get(edge.path) || createGraphEdge(edge.path), edge, chain);
      updateGraphEdgeLabel(graphEdgeLabels.get(edge.path) || createGraphEdgeLabel(edge.path), edge, chain);
      seenEdges.add(edge.path);
    }
    pruneGraph(graphEdges, seenEdges, function (el) { el.remove(); });
    pruneGraph(graphEdgeLabels, seenEdges, function (bundle) { bundle.g.remove(); });

    // The first scan of a real estate runs for seconds with nothing to draw.
    // Both surfaces show a placeholder of the shape that is coming; the ticking
    // elapsed count in the banner is what says it is still working.
    var awaitingFirst = !sawFirstSnapshot && model.scanState === 'scanning';
    setHidden(graphSkeletonEl, !awaitingFirst);
    setHidden(railSkeletonEl, !awaitingFirst);
    graphEmptyEl.hidden = layout.nodes.length > 0 || awaitingFirst;
    graphEmptyTextEl.textContent = 'Nothing to draw yet.';

    if (!graphFitted && layout.nodes.length > 0) {
      graphFitted = true;
      fitGraph();
    } else {
      applyGraphTransform();
    }
  }

  // Deferred to the end of render, never taken here: the dock is laid out
  // after the graph and shrinks the canvas under it, so a reveal measured at
  // this point aims at a viewport 136px taller than the one the reader ends up
  // with, and parks the node behind the dock.
  function takePendingGraphReveal() {
    if (!pendingGraphReveal) return;
    var path = pendingGraphReveal;
    pendingGraphReveal = null;
    if (graphSvgEl) revealGraphNode(path);
  }

  // ---------------------------------------------------------------------
  // Pan, zoom and graph selection. One transform on one group, so hit
  // testing stays the browser's problem and a push never touches it.
  // ---------------------------------------------------------------------

  function applyGraphTransform() {
    var z = view.zoom;
    scheduleUrlSync();
    graphViewportEl.setAttribute('transform',
      'translate(' + z.x + ',' + z.y + ') scale(' + z.k + ')');
    var minimal = z.k < GRAPH_LABEL_MIN_K;
    graphSvgEl.classList.toggle('graph-svg--minimal', minimal);
    graphSvgEl.setAttribute('data-duration-floor', String(graphDurationFloor(z.k)));
    graphSvgEl.style.setProperty('--graph-label-scale', String(minimal ? GRAPH_LABEL_MIN_K / z.k : 1));
    graphZoomReadoutEl.textContent = Math.round(z.k * 100) + '%';
    updateGraphScrollFade();
  }

  // Shown only while laid-out content sits below the visible canvas, gone once
  // the last row is in view: the rail's rule, applied to the one surface that
  // has no scrollbar to say the same thing.
  function updateGraphScrollFade() {
    if (!graphSvgEl || !graphScrollFadeEl) return;
    var z = view.zoom;
    var below = (z.y + graphExtent.height * z.k) - graphSvgEl.clientHeight > 1;
    graphScrollFadeEl.classList.toggle('graph-scroll-fade--visible', below);
  }

  function zoomGraphAt(cx, cy, factor) {
    var z = view.zoom;
    var k = clamp(z.k * factor, GRAPH_MIN_K, GRAPH_MAX_K);
    if (k === z.k) return;
    var ratio = k / z.k;
    z.x = cx - (cx - z.x) * ratio;
    z.y = cy - (cy - z.y) * ratio;
    z.k = k;
    applyGraphTransform();
  }

  function zoomGraphAtCentre(factor) {
    zoomGraphAt(graphSvgEl.clientWidth / 2, graphSvgEl.clientHeight / 2, factor);
  }

  function fitGraph() {
    var vw = graphSvgEl.clientWidth;
    var vh = graphSvgEl.clientHeight;
    if (!vw || !vh || !graphExtent.width || !graphExtent.height) return;
    var k = Math.min((vw - 2 * GRAPH_FIT_PAD) / graphExtent.width,
      (vh - 2 * GRAPH_FIT_PAD) / graphExtent.height);
    // Never below the label threshold: a first paint with every name dropped
    // is a worse trade than a canvas the viewer has to pan.
    view.zoom.k = clamp(k, GRAPH_LABEL_MIN_K, 1);
    var scaled = graphExtent.height * view.zoom.k;
    view.zoom.x = GRAPH_FIT_PAD;
    view.zoom.y = scaled < vh ? (vh - scaled) / 2 : GRAPH_FIT_PAD;
    applyGraphTransform();
  }

  // Cancelled by any new target and by the user's own pan or zoom, so a
  // running tween can never fight the pointer.
  var graphPanFrame = null;

  function stopGraphPan() {
    if (graphPanFrame !== null) {
      window.cancelAnimationFrame(graphPanFrame);
      graphPanFrame = null;
    }
  }

  function panGraphTo(tx, ty, tk) {
    stopGraphPan();
    var z = view.zoom;
    var reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (reduced || (Math.abs(tx - z.x) < 1 && Math.abs(ty - z.y) < 1 && Math.abs(tk - z.k) < 0.01)) {
      z.x = tx; z.y = ty; z.k = tk;
      applyGraphTransform();
      return;
    }
    var fromX = z.x, fromY = z.y, fromK = z.k;
    var start = null;
    function step(now) {
      if (start === null) start = now;
      var t = Math.min((now - start) / GRAPH_FOLLOW_MS, 1);
      var e = 1 - Math.pow(1 - t, 3);
      z.x = fromX + (tx - fromX) * e;
      z.y = fromY + (ty - fromY) * e;
      z.k = fromK + (tk - fromK) * e;
      applyGraphTransform();
      graphPanFrame = t < 1 ? window.requestAnimationFrame(step) : null;
    }
    graphPanFrame = window.requestAnimationFrame(step);
  }

  // Puts the node in the middle of the canvas. Zoom is only ever raised, so
  // following never pulls back from a level the viewer chose.
  function syncFollowButton() {
    if (!graphFollowEl) return;
    graphFollowEl.setAttribute('aria-pressed', String(view.follow));
    graphFollowEl.setAttribute('title', view.follow
      ? 'Following the selection. Click to stop'
      : 'Follow selection');
    graphFollowEl.setAttribute('aria-label', graphFollowEl.getAttribute('title'));
  }

  function centreGraphNode(path) {
    var node = graphPositions.get(path);
    if (!node) return;
    var k = Math.min(Math.max(view.zoom.k, GRAPH_FOLLOW_MIN_K), GRAPH_MAX_K);
    var cx = node.x + node.width / 2;
    var cy = node.y;
    panGraphTo(graphSvgEl.clientWidth / 2 - cx * k, graphSvgEl.clientHeight / 2 - cy * k, k);
  }

  function revealGraphNode(path) {
    if (view.follow) {
      centreGraphNode(path);
      return;
    }
    var node = graphPositions.get(path);
    if (!node) return;
    var z = view.zoom;
    var pad = 36;
    var vw = graphSvgEl.clientWidth;
    var vh = graphSvgEl.clientHeight;
    var left = z.x + node.x * z.k;
    var right = z.x + (node.x + node.width) * z.k;
    var top = z.y + (node.y - GRAPH_NODE_HEIGHT / 2) * z.k;
    var bottom = z.y + (node.y + GRAPH_NODE_HEIGHT / 2) * z.k;

    if (left < pad) z.x += pad - left;
    else if (right > vw - pad) z.x -= Math.min(right - (vw - pad), left - pad);
    if (top < pad) z.y += pad - top;
    else if (bottom > vh - pad) z.y -= Math.min(bottom - (vh - pad), top - pad);
    applyGraphTransform();
  }

  // Clicking the selected node again clears it, so the graph can undo a
  // selection without reaching for the dock's close button or Escape.
  function selectFromGraph(path) {
    var repeat = view.selected === path;
    view.selected = repeat ? null : path;
    render();
    if (repeat) return;
    // A click already put the node on screen, so this only earns its keep
    // under follow, where expanding a container can move it afterwards.
    if (view.follow) centreGraphNode(path);
    var row = rows.get(path);
    // Follow centres both surfaces on the same node. Without it a nudge is
    // enough, since the row was already where the pointer was.
    if (row) row.row.scrollIntoView({ block: view.follow ? 'center' : 'nearest' });
  }

  // The rail's toggle, reached from the graph: the same expand set and the
  // same render, so a container opened on one surface is open on both. The
  // selection the first click made is left alone, and the reveal keeps the
  // node in view once its children have pushed the layout around.
  function toggleGraphExpansion(path) {
    var entry = model.index.get(path);
    if (!entry || entry.childPaths.length === 0) return;
    if (view.expanded.has(path)) view.expanded.delete(path); else view.expanded.add(path);
    pendingGraphReveal = path;
    render();
  }

  // Pointer events with capture, listeners on the captured element and an
  // end handler covering pointerup, pointercancel and lostpointercapture:
  // a drag abandoned outside the window must never leave the page pinned in
  // its dragging state.
  function wireGraphPointer(svg) {
    var panning = false;
    var pointerId = null;
    var startX = 0;
    var startY = 0;
    var originX = 0;
    var originY = 0;
    var moved = 0;
    var pressedPath = null;
    // Counted here rather than from a dblclick listener: pointer capture
    // retargets the compatibility mouse events to the svg, so a dblclick
    // handler cannot tell which node was under the pointer, while the
    // pointerdown target can.
    var lastClickPath = null;
    var lastClickAt = 0;

    function onMove(e) {
      if (!panning) return;
      var dx = e.clientX - startX;
      var dy = e.clientY - startY;
      moved = Math.max(moved, Math.abs(dx), Math.abs(dy));
      view.zoom.x = originX + dx;
      view.zoom.y = originY + dy;
      applyGraphTransform();
    }

    function endPan() {
      if (!panning) return;
      panning = false;
      svg.classList.remove('graph-svg--panning');
      svg.removeEventListener('pointermove', onMove);
      svg.removeEventListener('pointerup', endPan);
      svg.removeEventListener('pointercancel', endPan);
      svg.removeEventListener('lostpointercapture', endPan);
      if (pointerId !== null && svg.hasPointerCapture(pointerId)) svg.releasePointerCapture(pointerId);
      pointerId = null;
      // A press that never really moved is a click. Resolved from the
      // pointerdown target's dataset, so the drag and the selection cannot
      // disagree about which node was under the pointer.
      if (moved <= GRAPH_CLICK_SLOP && pressedPath && pressedPath !== ROOT_PATH) {
        var now = Date.now();
        var second = pressedPath === lastClickPath && now - lastClickAt <= GRAPH_DOUBLE_CLICK_MS;
        // Zeroed on the second press so a third does not toggle straight back.
        lastClickPath = pressedPath;
        lastClickAt = second ? 0 : now;
        if (second) toggleGraphExpansion(pressedPath);
        else selectFromGraph(pressedPath);
      }
      pressedPath = null;
    }

    svg.addEventListener('pointerdown', function (e) {
      if (e.button !== 0 && e.button !== 1) return;
      e.preventDefault();
      stopGraphPan();
      svg.focus();
      var group = e.target.closest ? e.target.closest('.graph-node') : null;
      pressedPath = group ? group.dataset.path : null;
      panning = true;
      pointerId = e.pointerId;
      moved = 0;
      startX = e.clientX;
      startY = e.clientY;
      originX = view.zoom.x;
      originY = view.zoom.y;
      svg.classList.add('graph-svg--panning');
      svg.setPointerCapture(e.pointerId);
      svg.addEventListener('pointermove', onMove);
      svg.addEventListener('pointerup', endPan);
      svg.addEventListener('pointercancel', endPan);
      svg.addEventListener('lostpointercapture', endPan);
    });

    // Plain wheel pans, ctrl or meta wheel zooms, which is also what a
    // trackpad pinch sends. passive:false because both branches take over
    // from the browser's own scrolling.
    svg.addEventListener('wheel', function (e) {
      e.preventDefault();
      stopGraphPan();
      if (e.ctrlKey || e.metaKey) {
        var rect = svg.getBoundingClientRect();
        // Clamped before the exponential: a pinch sends many small deltas, a
        // wheel notch sends one of 120, and unclamped that single notch would
        // cross the whole zoom range at once.
        zoomGraphAt(e.clientX - rect.left, e.clientY - rect.top,
          Math.exp(-clamp(e.deltaY, -40, 40) * 0.01));
        return;
      }
      view.zoom.x -= e.deltaX;
      view.zoom.y -= e.deltaY;
      applyGraphTransform();
    }, { passive: false });

    svg.addEventListener('keydown', function (e) {
      var step = 48;
      if (e.key === '+' || e.key === '=') zoomGraphAtCentre(1.25);
      else if (e.key === '-' || e.key === '_') zoomGraphAtCentre(1 / 1.25);
      else if (e.key === '0') zoomGraphAtCentre(1 / view.zoom.k);
      else if (e.key === 'f' || e.key === 'F') fitGraph();
      else if (e.key === 'ArrowLeft') view.zoom.x += step;
      else if (e.key === 'ArrowRight') view.zoom.x -= step;
      else if (e.key === 'ArrowUp') view.zoom.y += step;
      else if (e.key === 'ArrowDown') view.zoom.y -= step;
      else return;
      e.preventDefault();
      applyGraphTransform();
    });
  }

  // ---------------------------------------------------------------------
  // Fullscreen: the canvas alone, not the whole app, so the topology gets
  // the screen without the rail and the dock competing for it. The button
  // is a child of the element that goes fullscreen, or it would disappear
  // exactly when it is the only way back out.
  // ---------------------------------------------------------------------

  var FULLSCREEN_ENTER_ICON = '\u21f2';
  var FULLSCREEN_EXIT_ICON = '\u21f1';

  // Safari answers to the webkit spellings of all four of these, and a
  // browser with none of them gets no button at all.
  function fullscreenRequestFn(el) {
    return el.requestFullscreen || el.webkitRequestFullscreen || null;
  }

  function fullscreenExitFn() {
    return document.exitFullscreen || document.webkitExitFullscreen || null;
  }

  function currentFullscreenElement() {
    return document.fullscreenElement || document.webkitFullscreenElement || null;
  }

  function fullscreenSupported(el) {
    return Boolean(fullscreenRequestFn(el)) && Boolean(fullscreenExitFn());
  }

  function wireFullscreen(button, target) {
    function sync() {
      var active = currentFullscreenElement() === target;
      var label = active ? 'Exit fullscreen' : 'Fullscreen';
      button.textContent = active ? FULLSCREEN_EXIT_ICON : FULLSCREEN_ENTER_ICON;
      button.setAttribute('aria-pressed', String(active));
      button.setAttribute('aria-label', label);
      button.setAttribute('title', label);
      // The whole reflow: the extent is layout space and fitGraph measures
      // the box when it is asked, so nothing viewport-derived is cached and
      // re-applying the one transform carries pan, zoom, selection and the
      // route through the resize untouched.
      applyGraphTransform();
    }

    // requestFullscreen rejects when permissions policy blocks it, and the
    // webkit form returns undefined instead of a promise. Either way the
    // button re-reads the real state rather than asserting what it asked for.
    function invoke(fn, context) {
      var result;
      try {
        result = fn.call(context);
      } catch (e) {
        sync();
        return;
      }
      if (result && typeof result.catch === 'function') result.catch(sync);
    }

    button.addEventListener('click', function () {
      if (currentFullscreenElement() === target) {
        invoke(fullscreenExitFn(), document);
        return;
      }
      invoke(fullscreenRequestFn(target), target);
    });

    // State follows the event, never the click: Esc, the browser's own
    // control and an app switch all leave fullscreen without coming through
    // the handler above, and a button that counted its own clicks would lie
    // from the first Esc onward.
    document.addEventListener('fullscreenchange', sync);
    document.addEventListener('webkitfullscreenchange', sync);
    sync();
  }

  // ---------------------------------------------------------------------
  // The keyed list reconciler the dock, the banners and both popovers share.
  // Same discipline as the rail and the graph: create once per key, update in
  // place, remove what the new spec no longer names. An element already in
  // position is never re-inserted, so a push cannot blur focus or reset a
  // scroll position under the reader.
  // ---------------------------------------------------------------------

  function keyedList(container, create) {
    var store = new Map();
    return function (specs, update) {
      var seen = new Set();
      for (var i = 0; i < specs.length; i++) {
        var spec = specs[i];
        var bundle = store.get(spec.key);
        if (!bundle) {
          bundle = create(spec);
          store.set(spec.key, bundle);
        }
        update(bundle, spec);
        seen.add(spec.key);
        var atPosition = container.childNodes[i];
        if (atPosition !== bundle.el) container.insertBefore(bundle.el, atPosition || null);
      }
      store.forEach(function (bundle, key) {
        if (seen.has(key)) return;
        bundle.el.remove();
        store.delete(key);
      });
    };
  }

  function element(tag, className, text) {
    var el = document.createElement(tag);
    if (className) el.className = className;
    if (text !== undefined) el.textContent = text;
    return el;
  }

  function fieldList(container) {
    var sync = keyedList(container, function () {
      var el = element('div', 'dock-field');
      var label = element('span', 'dock-field-label');
      var value = element('span', 'dock-field-value');
      el.appendChild(label);
      el.appendChild(value);
      return { el: el, label: label, value: value };
    });
    return function (fields) {
      container.hidden = fields.length === 0;
      sync(fields, function (bundle, spec) {
        bundle.label.textContent = spec.label;
        bundle.value.className = 'dock-field-value' + (spec.tone ? ' dock-tone--' + spec.tone : '');
        if (!spec.crumbs) {
          bundle.value.textContent = spec.value;
          return;
        }
        // Rebuilt rather than reconciled: a trail is at most a handful of
        // nodes and changes only when the selection does.
        bundle.value.replaceChildren();
        for (var i = 0; i < spec.crumbs.length; i++) {
          if (i > 0) bundle.value.appendChild(document.createTextNode(' / '));
          var crumb = spec.crumbs[i];
          // The last crumb is the selected component itself, so it is text.
          if (i === spec.crumbs.length - 1) {
            bundle.value.appendChild(document.createTextNode(crumb.name));
            continue;
          }
          var button = element('button', 'dock-crumb');
          button.type = 'button';
          button.textContent = crumb.name;
          button.dataset.path = crumb.key;
          button.title = 'Select ' + crumb.name;
          bundle.value.appendChild(button);
        }
      });
    };
  }

  function groupList(container) {
    var sync = keyedList(container, function () {
      var el = element('div', 'dock-group');
      var label = element('h4', 'dock-group-label');
      var list = element('ul', 'dock-lines');
      el.appendChild(label);
      el.appendChild(list);
      return { el: el, label: label, list: list, sync: keyedList(list, function () {
        var li = element('li', 'dock-line');
        return { el: li };
      }) };
    });
    return function (groups) {
      container.hidden = groups.length === 0;
      sync(groups, function (bundle, spec) {
        bundle.label.textContent = spec.label + ' (' + spec.rows.length + ')';
        bundle.sync(spec.rows, function (row, rowSpec) {
          row.el.textContent = rowSpec.text;
          row.el.className = 'dock-line' + (rowSpec.tone ? ' dock-tone--' + rowSpec.tone : '');
        });
      });
    };
  }

  // ---------------------------------------------------------------------
  // The detail dock: one summary line that expands to messages and rendered
  // details. Its elements are built once and detached whole when nothing is
  // selected, because .detail-dock:empty is what keeps an unselected dock
  // from stealing height from the rail.
  // ---------------------------------------------------------------------

  function buildDock() {
    var inner = element('div', 'dock-inner');

    var summary = element('div', 'dock-summary');
    var toggle = element('button', 'dock-toggle');
    toggle.type = 'button';
    var dot = element('span', 'status-dot');
    var name = element('span', 'dock-name');
    var type = element('span', 'dock-type chip');
    var status = element('span', 'dock-status');
    var duration = element('span', 'dock-duration');
    var meta = element('span', 'dock-meta');
    var spacer = element('span', 'dock-spacer');
    var close = element('button', 'dock-close', '×');
    close.type = 'button';
    close.setAttribute('aria-label', 'Clear selection');
    close.setAttribute('title', 'Clear selection');

    summary.appendChild(toggle);
    summary.appendChild(dot);
    summary.appendChild(name);
    summary.appendChild(type);
    summary.appendChild(status);
    summary.appendChild(duration);
    summary.appendChild(meta);
    summary.appendChild(spacer);
    summary.appendChild(close);

    var body = element('div', 'dock-body');
    var about = element('div', 'dock-fields');
    var messages = element('div', 'dock-section');
    var messagesLabel = element('h3', 'dock-section-label');
    var messagesList = element('ul', 'dock-lines');
    messages.appendChild(messagesLabel);
    messages.appendChild(messagesList);
    var cards = element('div', 'dock-cards');
    body.appendChild(about);
    body.appendChild(messages);
    body.appendChild(cards);

    var fade = element('div', 'dock-fade');
    var more = element('div', 'dock-more', 'more below');

    inner.appendChild(summary);
    inner.appendChild(body);
    inner.appendChild(fade);
    inner.appendChild(more);

    toggle.addEventListener('click', function () {
      view.dockOpen = !view.dockOpen;
      render();
    });
    close.addEventListener('click', function () {
      view.selected = null;
      render();
    });
    body.addEventListener('scroll', updateDockOverflow);

    // Delegated: crumbs are rebuilt whenever the selection changes.
    inner.addEventListener('click', function (e) {
      var crumb = e.target.closest ? e.target.closest('.dock-crumb') : null;
      if (crumb && crumb.dataset.path) selectPath(crumb.dataset.path);
    });

    var syncCards = keyedList(cards, function () {
      var el = element('article', 'dock-card');
      var head = element('header', 'dock-card-head');
      var title = element('h3', 'dock-card-title');
      var tag = element('span', 'dock-card-tag', 'no renderer');
      head.appendChild(title);
      head.appendChild(tag);
      var fields = element('div', 'dock-fields');
      var groups = element('div', 'dock-groups');
      el.appendChild(head);
      el.appendChild(fields);
      el.appendChild(groups);
      return {
        el: el, title: title, tag: tag,
        syncFields: fieldList(fields), syncGroups: groupList(groups)
      };
    });

    return {
      inner: inner, summary: summary, toggle: toggle, dot: dot, name: name,
      type: type, status: status, duration: duration, meta: meta, close: close,
      body: body, messages: messages, messagesLabel: messagesLabel,
      syncAbout: fieldList(about),
      syncMessages: keyedList(messagesList, function () {
        return { el: element('li', 'dock-line') };
      }),
      syncCards: syncCards,
      fade: fade, more: more
    };
  }

  // The dock is short and scrolls, so it has to say so. A silently scrollable
  // region that looks like it ends is the complaint this exists to answer.
  function updateDockOverflow() {
    if (!dock || !dock.inner.parentNode) return;
    var body = dock.body;
    var hasMore = body.scrollHeight - body.scrollTop - body.clientHeight > 1;
    dock.fade.classList.toggle('dock-fade--visible', hasMore);
    dock.more.hidden = !hasMore;
  }

  function dockMetaText(spec) {
    var parts = [];
    if (spec.messages.length) {
      parts.push(spec.messages.length + (spec.messages.length === 1 ? ' message' : ' messages'));
    }
    if (spec.details.length) {
      parts.push(spec.details.length + (spec.details.length === 1 ? ' detail' : ' details'));
    }
    if (spec.childCount) parts.push(spec.childCount + ' children');
    if (spec.unhealthyDescendants) parts.push(spec.unhealthyDescendants + ' unhealthy below');
    if (spec.failFast) parts.push('fail-fast triggered');
    return parts.join(' · ');
  }

  function renderDock() {
    if (!dockEl) return;
    var spec = view.selected ? describeDock(model.index, view.selected) : null;
    if (!spec) {
      if (dock && dock.inner.parentNode) dock.inner.remove();
      return;
    }
    if (!dock) dock = buildDock();
    if (!dock.inner.parentNode) dockEl.appendChild(dock.inner);

    dock.inner.dataset.path = spec.path;
    dock.dot.className = 'status-dot status-dot--' + statusClass(spec.status);
    dock.name.textContent = spec.name;
    dock.type.textContent = displayType(spec.type);
    dock.type.hidden = !spec.type;
    if (spec.type) dock.type.setAttribute('title', spec.type);
    dock.status.textContent = spec.status.toLowerCase().replace(/_/g, ' ');
    dock.status.className = 'dock-status dock-status--' + statusClass(spec.status);
    dock.duration.textContent = spec.duration;
    dock.meta.textContent = dockMetaText(spec);

    var expandable = spec.messages.length > 0 || spec.details.length > 0 || spec.trail.length > 1 ||
      Boolean(spec.serverId) || spec.failFast;
    var open = expandable && view.dockOpen;
    dock.toggle.textContent = open ? '▾' : '▸';
    dock.toggle.hidden = !expandable;
    dock.toggle.setAttribute('aria-expanded', String(open));
    dock.toggle.setAttribute('aria-label', open ? 'Collapse detail' : 'Expand detail');
    dock.toggle.setAttribute('title', open ? 'Collapse detail' : 'Expand detail');
    dock.body.hidden = !open;

    dock.syncAbout(compact([
      crumbField('path', 'Path', spec.crumbs),
      field('server', 'Satellite server', spec.serverId),
      spec.failFast ? field('failfast', 'Fail-fast', 'triggered here, so later checks were skipped', 'error') : null
    ]));

    dock.messages.hidden = spec.messages.length === 0;
    dock.messagesLabel.textContent = 'Messages (' + spec.messages.length + ')';
    dock.syncMessages(spec.messages.map(function (message, i) {
      return { key: 'm' + i, text: message };
    }), function (bundle, rowSpec) {
      bundle.el.textContent = rowSpec.text;
    });

    dock.syncCards(spec.details, function (bundle, detail) {
      bundle.title.textContent = detail.title;
      bundle.el.setAttribute('title', detail.typeName);
      bundle.tag.hidden = detail.known;
      bundle.syncFields(detail.fields);
      bundle.syncGroups(detail.groups);
    });

    updateDockOverflow();
  }

  // ---------------------------------------------------------------------
  // Banners: every degraded state, rendered from computeBanners, above the
  // rail and the canvas rather than tucked into either of them.
  // ---------------------------------------------------------------------

  function buildBanners(shell) {
    var el = element('div', 'banners');
    shell.insertBefore(el, shell.firstChild);
    var sync = keyedList(el, function () {
      var banner = element('div', 'banner');
      var mark = element('span', 'banner-mark');
      var text = element('div', 'banner-text');
      var title = element('strong', 'banner-title');
      var body = element('span', 'banner-body');
      var actions = element('div', 'banner-actions');
      text.appendChild(title);
      text.appendChild(body);
      banner.appendChild(mark);
      banner.appendChild(text);
      banner.appendChild(actions);
      return {
        el: banner, mark: mark, title: title, body: body,
        syncActions: keyedList(actions, function () {
          var button = element('button', 'banner-action');
          button.type = 'button';
          return { el: button };
        }),
        actions: actions
      };
    });
    return { el: el, sync: sync };
  }

  var BANNER_MARKS = { error: '●', warn: '▲', info: '○' };

  function renderBanners() {
    if (!syncBanners) return;
    var specs = computeBanners({
      index: model.index,
      connection: model.connection,
      lastError: model.lastError,
      triggerError: model.triggerError,
      scanState: model.scanState,
      scanStartedAt: model.scanStartedAt,
      streamState: model.streamState,
      streamStopped: model.streamStopped,
      haveSnapshot: sawFirstSnapshot,
      componentCount: Math.max(model.index.size - 1, 0)
    });
    bannersEl.hidden = specs.length === 0;
    syncBanners(specs, function (bundle, spec) {
      bundle.el.className = 'banner banner--' + spec.severity;
      bundle.el.setAttribute('role', spec.severity === 'info' ? 'status' : 'alert');
      bundle.mark.textContent = BANNER_MARKS[spec.severity] || '○';
      bundle.title.textContent = spec.title;
      bundle.body.textContent = spec.body;
      var entries = spec.entries || [];
      bundle.actions.hidden = entries.length === 0;
      bundle.syncActions(entries.map(function (entry) {
        return { key: entry.path, path: entry.path, label: entry.name };
      }), function (action, actionSpec) {
        action.el.textContent = 'Show ' + actionSpec.label;
        action.el.dataset.path = actionSpec.path;
        if (!action.wired) {
          action.wired = true;
          action.el.addEventListener('click', function () {
            selectPath(action.el.dataset.path);
          });
        }
      });
    });
  }

  // ---------------------------------------------------------------------
  // Header popovers: the unhealthy pill and the liveness readout. Hover
  // expands, a click pins it open so the pointer is free, Escape closes and
  // hands focus back. Both are keyed lists of buttons, so Tab reaches every
  // entry and a push never rebuilds what is under the pointer.
  // ---------------------------------------------------------------------

  var openPopover = null;

  function createPopover(anchor, className, label) {
    var panel = element('div', 'popover ' + className);
    panel.hidden = true;
    panel.setAttribute('role', 'group');
    panel.setAttribute('aria-label', label);
    var list = element('div', 'popover-list');
    var empty = element('p', 'popover-empty');
    panel.appendChild(list);
    panel.appendChild(empty);
    document.body.appendChild(panel);

    anchor.setAttribute('role', 'button');
    anchor.setAttribute('tabindex', '0');
    anchor.setAttribute('aria-haspopup', 'true');
    anchor.setAttribute('aria-expanded', 'false');
    anchor.classList.add('has-popover');

    var pinned = false;
    var overAnchor = false;
    var overPanel = false;
    var closeTimer = null;

    function position() {
      var rect = anchor.getBoundingClientRect();
      var width = panel.offsetWidth;
      var left = Math.min(rect.left, Math.max(8, window.innerWidth - width - 8));
      panel.style.setProperty('left', Math.max(8, left) + 'px');
      panel.style.setProperty('top', (rect.bottom + 6) + 'px');
      panel.style.setProperty('max-height', (window.innerHeight - rect.bottom - 24) + 'px');
    }

    function show() {
      if (openPopover && openPopover !== api) openPopover.close();
      openPopover = api;
      panel.hidden = false;
      anchor.setAttribute('aria-expanded', 'true');
      position();
    }

    function close() {
      pinned = false;
      panel.hidden = true;
      anchor.setAttribute('aria-expanded', 'false');
      if (openPopover === api) openPopover = null;
    }

    function scheduleClose() {
      if (closeTimer) clearTimeout(closeTimer);
      // A short grace, or the gap between the anchor and the panel closes it
      // before the pointer can cross.
      closeTimer = setTimeout(function () {
        closeTimer = null;
        if (!pinned && !overAnchor && !overPanel) close();
      }, 180);
    }

    anchor.addEventListener('mouseenter', function () {
      overAnchor = true;
      show();
    });
    anchor.addEventListener('mouseleave', function () {
      overAnchor = false;
      scheduleClose();
    });
    panel.addEventListener('mouseenter', function () { overPanel = true; });
    panel.addEventListener('mouseleave', function () {
      overPanel = false;
      scheduleClose();
    });

    anchor.addEventListener('click', function () {
      if (pinned) {
        close();
        return;
      }
      pinned = true;
      show();
    });
    anchor.addEventListener('keydown', function (e) {
      if (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar') return;
      e.preventDefault();
      if (pinned) {
        close();
        return;
      }
      pinned = true;
      show();
    });

    var api = {
      panel: panel, list: list, empty: empty,
      close: close,
      isOpen: function () { return !panel.hidden; },
      reposition: function () { if (!panel.hidden) position(); },
      focusAnchor: function () { anchor.focus(); }
    };
    return api;
  }

  function popoverEntryList(popover) {
    return keyedList(popover.list, function () {
      var button = element('button', 'popover-entry');
      button.type = 'button';
      var dot = element('span', 'status-dot');
      var name = element('span', 'popover-entry-name');
      var trail = element('span', 'popover-entry-trail');
      button.appendChild(dot);
      button.appendChild(name);
      button.appendChild(trail);
      button.addEventListener('click', function () {
        popover.close();
        selectPath(button.dataset.path);
      });
      return { el: button, dot: dot, name: name, trail: trail };
    });
  }

  var syncPillEntries = null;
  var syncTransitionEntries = null;

  function renderPill() {
    var unhealthy = collectUnhealthy(model.index);
    pillEl.textContent = unhealthy.length + ' unhealthy';
    pillEl.hidden = unhealthy.length === 0;
    if (unhealthy.length === 0 && pillPopover) pillPopover.close();
    if (!pillPopover) return;

    pillPopover.empty.hidden = unhealthy.length > 0;
    pillPopover.empty.textContent = 'Nothing unhealthy.';
    syncPillEntries(unhealthy.map(function (entry) {
      return { key: entry.path, entry: entry };
    }), function (bundle, spec) {
      var entry = spec.entry;
      bundle.el.dataset.path = entry.path;
      bundle.dot.className = 'status-dot status-dot--' + statusClass(entry.status);
      bundle.name.textContent = entry.name;
      bundle.trail.textContent = entry.trail.slice(0, -1).join(' / ');
      bundle.trail.hidden = entry.trail.length < 2;
    });
  }

  function transitionLabel(entry) {
    if (!entry.from) return 'appeared as ' + entry.to.toLowerCase();
    if (!entry.to) return 'removed while ' + entry.from.toLowerCase();
    return entry.from.toLowerCase() + ' → ' + entry.to.toLowerCase();
  }

  function renderTransitions() {
    if (!livenessPopover) return;
    var log = model.transitionLog;
    livenessPopover.empty.hidden = log.length > 0;
    livenessPopover.empty.textContent = model.isFirstSnapshot && log.length === 0
      ? 'No status changes since this tab connected. The first scan is a baseline, not a change.'
      : 'No status changes yet.';
    syncTransitionEntries(log.map(function (entry) {
      return { key: entry.key, entry: entry };
    }), function (bundle, spec) {
      var entry = spec.entry;
      bundle.el.dataset.path = entry.path;
      bundle.dot.className = 'status-dot status-dot--' + statusClass(entry.to || entry.from);
      bundle.name.textContent = entry.trail.length ? entry.trail[entry.trail.length - 1] : entry.path;
      bundle.trail.textContent = transitionLabel(entry) + '  ·  ' + entry.at.toLocaleTimeString();
      bundle.trail.hidden = false;
      bundle.el.disabled = !model.index.has(entry.path);
    });
  }

  // ---------------------------------------------------------------------
  // Liveness. "Sampled", never anything implying continuity: a flap between
  // two polls is undetectable by any design here, and the line must not
  // pretend otherwise.
  // ---------------------------------------------------------------------

  function samplingText() {
    if (model.refreshMs > 0) return 'sampled every ' + formatInterval(model.refreshMs);
    return 'sampled on demand only';
  }

  function updateLiveness() {
    var parts = [];
    if (model.scanState === 'scanning') {
      var elapsed = elapsedSeconds(model.scanStartedAt);
      // The one thing on screen that moves during a first scan of a live
      // estate, which is eight seconds of otherwise identical pixels.
      var running = (model.queuedFollowUp ? 'scanning, refresh queued' : 'scanning') +
        (elapsed === null ? '' : ' for ' + elapsed + 's');
      parts.push(running);
    } else if (model.lastScanAt) {
      parts.push('last checked ' + formatAge(Date.now() - model.lastScanAt.getTime()));
    } else {
      parts.push('never scanned');
    }
    parts.push(samplingText());
    // Suppressed mid-scan: an idle channel and a running scan are both true
    // for the moment before the RPC wakes the channel, and printing the pair
    // reads as a contradiction rather than as two facts.
    if (model.scanState !== 'scanning' && model.connection && model.connection.severity === 'benign') {
      parts.push('channel idle');
    }
    if (model.transitionLog.length) {
      parts.push(model.transitionLog.length +
        (model.transitionLog.length === 1 ? ' change' : ' changes'));
    }
    lastScanEl.textContent = parts.join(' · ');
    lastScanEl.setAttribute('title', 'Status changes since this tab connected');
  }

  // Driven from model.scanState, never from the click: an auto-refresh and a
  // press are the same running scan, and the button has to say so either way.
  function updateScanButton() {
    var scanning = model.scanState === 'scanning';
    var label = scanning ? (model.queuedFollowUp ? 'Queued' : 'Scanning') : 'Scan now';
    if (scanButton.textContent !== label) scanButton.textContent = label;
    scanButton.classList.toggle('scan-now--busy', scanning);
    scanButton.setAttribute('aria-busy', String(scanning));
  }

  // ---------------------------------------------------------------------
  // Selection from anywhere but the rail: the popovers and the banners name
  // a path the tree may have collapsed, and a selection with no node on
  // either surface is a selection nobody can see.
  // ---------------------------------------------------------------------

  function expandAncestors(path) {
    var entry = model.index.get(path);
    if (!entry) return;
    var ancestor = entry.parentPath;
    while (ancestor !== null && ancestor !== ROOT_PATH) {
      var ancestorEntry = model.index.get(ancestor);
      if (!ancestorEntry) break;
      if (ancestorEntry.childPaths.length > 0) view.expanded.add(ancestor);
      ancestor = ancestorEntry.parentPath;
    }
  }

  function selectPath(path) {
    if (!path || !model.index.has(path)) return;
    expandAncestors(path);
    view.selected = path;
    view.dockOpen = true;
    pendingGraphReveal = path;
    render();
    var row = rows.get(path);
    // Same rule as a graph click: follow centres, otherwise nudge.
    if (row) row.row.scrollIntoView({ block: view.follow ? 'center' : 'nearest' });
  }

  // ---------------------------------------------------------------------
  // Ingest: snapshot handling and keyed reconciliation.
  // ---------------------------------------------------------------------

  function handleSnapshot(payload) {
    var root = payload.snapshot || {};
    var newIndex = buildIndex(root);
    var oldPaths = new Set(model.index.keys());
    var diff = diffPaths(oldPaths, newIndex);

    for (var i = 0; i < diff.removed.length; i++) {
      var removedPath = diff.removed[i];
      view.expanded.delete(removedPath);
      if (view.selected === removedPath) view.selected = null;
      destroyRow(removedPath);
    }

    var isFirst = !sawFirstSnapshot;
    seedDefaultExpansion(newIndex, diff.added);
    // The first scan's transitions are all from-empty appearances, not
    // changes: only here, never again, do we also force unhealthy paths
    // open regardless of the size collapse policy.
    if (isFirst) expandAncestorsOfUnhealthy(newIndex);

    model.index = newIndex;
    model.scanID = payload.scanID;
    model.seq = payload.seq;
    model.observedAt = payload.observedAt;
    model.transitions = payload.transitions || [];
    model.isFirstSnapshot = isFirst;
    var firstSnapshot = !sawFirstSnapshot;
    sawFirstSnapshot = true;
    // A replayed snapshot arrives without the scan event that produced it, so
    // the liveness line would say "never scanned" on every reconnect if it
    // waited for one. Both carry the same observedAt.
    var observed = new Date(payload.observedAt);
    if (!Number.isNaN(observed.getTime()) &&
      (!model.lastScanAt || observed.getTime() > model.lastScanAt.getTime())) {
      model.lastScanAt = observed;
    }
    if (!isFirst) recordTransitions(model.transitions, payload.observedAt);

    render();
    // After the render, so the layout the link's viewport refers to exists.
    if (firstSnapshot) applyPendingUrlState();
  }

  // The first snapshot's transitions are all from-empty appearances, and are
  // suppressed by the caller. A replayed snapshot repeats the transitions of
  // the scan that produced it, so seq keys the log entry and a repeat lands
  // on the same key rather than doubling the list.
  function recordTransitions(transitions, observedAt) {
    if (!transitions.length) return;
    var at = new Date(observedAt);
    if (Number.isNaN(at.getTime())) at = new Date();
    var known = new Set(model.transitionLog.map(function (entry) { return entry.key; }));
    var added = [];
    for (var i = 0; i < transitions.length; i++) {
      var t = transitions[i];
      var key = model.seq + '|' + t.path;
      if (known.has(key)) continue;
      added.push({
        key: key, path: t.path, from: t.from || '', to: t.to || '',
        trail: pathNames(t.path), at: at
      });
    }
    model.transitionLog = added.reverse().concat(model.transitionLog).slice(0, TRANSITION_LOG_LIMIT);
  }

  function handleConnection(payload) {
    if (!payload) return;
    model.connection = payload;
    if (typeof payload.refreshMs === 'number') model.refreshMs = payload.refreshMs;
    if (payload.target) targetEl.textContent = payload.target;
    // The graph's root node is labelled from the target element, so a target
    // that lands after the first snapshot needs one more render to pick it up.
    render();
  }

  function handleScan(payload) {
    model.scanState = 'idle';
    model.queuedFollowUp = false;
    model.scanStartedAt = null;
    model.triggerError = null;
    model.lastError = null;
    if (payload && payload.observedAt) {
      var observed = new Date(payload.observedAt);
      if (!Number.isNaN(observed.getTime())) model.lastScanAt = observed;
    }
    model.lastScanDurationMs = payload ? payload.durationMs : null;
    model.lastScanChanged = Boolean(payload && payload.changed);
    render();
  }

  function handleScanning(payload) {
    model.scanState = 'scanning';
    model.queuedFollowUp = Boolean(payload && payload.queuedFollowUp);
    // A replayed scanning frame can be seconds old, so the elapsed readout
    // starts from the server's startedAt, not from now.
    var started = payload && payload.startedAt ? new Date(payload.startedAt) : null;
    model.scanStartedAt = (started && !Number.isNaN(started.getTime())) ? started : new Date();
    model.triggerError = null;
    render();
  }

  function handleScanError(payload) {
    model.scanState = 'idle';
    model.queuedFollowUp = false;
    model.scanStartedAt = null;
    model.lastError = {
      code: (payload && payload.code) || 'Unknown',
      error: (payload && payload.error) || 'the scan failed with no message',
      at: payload && payload.at ? new Date(payload.at) : new Date()
    };
    render();
  }

  // Deliberate close, so EventSource must be told: left alone it reconnects
  // against a dead port every five seconds forever.
  function handleShutdown() {
    model.streamStopped = true;
    closeStream();
    render();
  }

  // A press that never reached the dashboard server must surface. Swallowed,
  // it looks exactly like a scan that is simply taking a while, which is the
  // one reading the viewer must not be left with.
  function triggerScan() {
    model.triggerError = null;
    render();
    fetch('/api/scan', { method: 'POST' })
      .then(function (response) {
        if (!response.ok) throw new Error('the server answered HTTP ' + response.status);
        return response.json();
      })
      .then(function (body) {
        // The server's own answer, not a guess: a press during a running scan
        // queues a follow-up rather than starting one.
        if (body && body.state === 'queued') model.queuedFollowUp = true;
        render();
      })
      .catch(function (error) {
        model.triggerError = (error && error.message) ? error.message : 'the request failed';
        render();
      });
  }

  function closeStream() {
    if (!es) return;
    es.close();
    es = null;
  }

  function connect() {
    if (es || model.streamStopped) return;
    es = new EventSource('/api/events');
    var handlers = {
      snapshot: handleSnapshot,
      scan: handleScan,
      scanning: handleScanning,
      'scan-error': handleScanError,
      connection: handleConnection,
      shutdown: handleShutdown
    };
    EVENT_NAMES.forEach(function (name) {
      es.addEventListener(name, function (e) {
        handlers[name](JSON.parse(e.data || '{}'));
      });
    });
    es.addEventListener('open', function () {
      model.streamState = 'open';
      render();
    });
    // readyState 2 is CLOSED, which EventSource reaches only on a fatal error
    // it will not retry. Anything else is its own reconnect in progress.
    es.addEventListener('error', function () {
      if (!es || model.streamStopped) return;
      model.streamState = es.readyState === 2 ? 'closed' : 'reconnecting';
      render();
    });
  }

  // ---------------------------------------------------------------------
  // Stream lifecycle. Closing on a hidden tab frees one of the browser's six
  // per-origin connections, a limit shared across every tab, and removes the
  // backgrounded-reader case that parks the server's write.
  // ---------------------------------------------------------------------

  var HIDDEN_GRACE_MS = 30000;
  var hiddenTimer = null;

  function wireStreamLifecycle() {
    document.addEventListener('visibilitychange', function () {
      if (document.hidden) {
        if (hiddenTimer) clearTimeout(hiddenTimer);
        hiddenTimer = setTimeout(function () {
          hiddenTimer = null;
          model.streamState = 'suspended';
          closeStream();
        }, HIDDEN_GRACE_MS);
        return;
      }
      if (hiddenTimer) {
        clearTimeout(hiddenTimer);
        hiddenTimer = null;
      }
      if (!es && !model.streamStopped) {
        model.streamState = 'connecting';
        connect();
        render();
      }
    });

    // pagehide, not unload: a page entering the back/forward cache never
    // fires unload, and its stream would stay open behind it.
    window.addEventListener('pagehide', function () { closeStream(); });
  }

  function init() {
    railEl = document.getElementById('tree-rail');
    var chrome = buildRailChrome(railEl);
    chipsEl = chrome.chips;
    treeRootEl = chrome.tree;
    railScrollFadeEl = chrome.fade;
    railEl.addEventListener('scroll', updateRailScrollFade);
    // Delegated, not one listener per row: the tree rebuilds as the estate
    // changes and there are hundreds of rows.
    treeRootEl.addEventListener('keydown', onTreeKeydown);
    treeRootEl.addEventListener('focusin', onTreeFocusIn);

    graphCanvasEl = document.getElementById('graph-canvas');
    var graph = buildGraphChrome(graphCanvasEl);
    graphSvgEl = graph.svg;
    graphViewportEl = graph.viewport;
    graphEdgeLayerEl = graph.edges;
    graphEdgeLabelLayerEl = graph.edgeLabels;
    graphNodeLayerEl = graph.nodes;
    graphEmptyEl = graph.empty;
    graphEmptyTextEl = graph.emptyText;
    graphFollowEl = graph.follow;
    graphScrollFadeEl = graph.fade;
    graphZoomReadoutEl = graph.readout;
    wireGraphPointer(graphSvgEl);
    applyGraphTransform();
    if (graph.fullscreen) wireFullscreen(graph.fullscreen, graphCanvasEl);

    dockEl = document.getElementById('detail-dock');
    var banners = buildBanners(document.querySelector('.shell'));
    bannersEl = banners.el;
    syncBanners = banners.sync;

    pillPopover = createPopover(pillEl, 'popover--unhealthy', 'Unhealthy components');
    syncPillEntries = popoverEntryList(pillPopover);
    livenessPopover = createPopover(lastScanEl, 'popover--transitions', 'Status changes');
    syncTransitionEntries = popoverEntryList(livenessPopover);

    scanButton.addEventListener('click', triggerScan);
    readUrlState();
    // After readUrlState, since a link carrying f=1 has to reach the button.
    syncFollowButton();
    // The filter came from the link, so the box has to show it.
    if (chrome.search && view.filter) chrome.search.value = view.filter;
    var resizeHandle = document.getElementById('rail-resize-handle');
    var railResize = resizeHandle ? wireRailResize(resizeHandle, document.documentElement) : null;
    window.addEventListener('resize', function () {
      updateRailScrollFade();
      updateDockOverflow();
      updateGraphScrollFade();
      if (pillPopover) pillPopover.reposition();
      if (livenessPopover) livenessPopover.reposition();
      if (railResize) railResize.reclampToViewport();
    });

    // Popover first, selection second: Escape dismisses the thing most
    // recently put on screen, and only when nothing is open does it fall
    // through to clearing the selection, which also drops the route highlight
    // and closes the dock.
    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') return;
      if (openPopover) {
        var popover = openPopover;
        popover.close();
        popover.focusAnchor();
        return;
      }
      // Escape in a text field belongs to the field.
      if (e.target && e.target.tagName === 'INPUT') return;
      if (!view.selected) return;
      view.selected = null;
      render();
    });
    // pointerdown, not click: a press outside should dismiss before whatever
    // it landed on runs, the same way a native menu behaves.
    document.addEventListener('pointerdown', function (e) {
      if (!openPopover || openPopover.panel.contains(e.target)) return;
      if (e.target === pillEl || e.target === lastScanEl) return;
      openPopover.close();
    });

    // One second, so "last checked 3s ago" is true when read rather than
    // when the scan landed. During a first scan the banner carries the same
    // count, and it is the only thing on a cold screen large enough to see.
    setInterval(function () {
      updateLiveness();
      if (!sawFirstSnapshot && model.scanState === 'scanning') renderBanners();
    }, 1000);

    wireStreamLifecycle();
    connect();
    render();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
