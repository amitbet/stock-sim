import { useEffect, useState } from "react";
import { indicatorTitle } from "../lib/indicatorScripts.js";

export default function ScriptsPanel({ scripts, scriptResults, loading, error, onToggleVisible, onEditScript }) {
  const [collapsed, setCollapsed] = useState(true);
  const activeCount = scripts.filter((script) => script.visible).length;
  return (
    <section className="panel scripts-panel">
      <button type="button" className="scripts-panel-toggle" aria-expanded={!collapsed} onClick={() => setCollapsed((value) => !value)}>
        <span>Scripts</span>
        <span className="scripts-panel-count">{activeCount} active</span>
        <span aria-hidden>{collapsed ? "▸" : "▾"}</span>
      </button>
      {!collapsed ? (
        <div className="scripts-list">
          {scripts.map((script) => {
            const result = scriptResults[script.id];
            return (
              <div className="script-row" key={script.id}>
                <label className="script-visible-toggle">
                  <input type="checkbox" checked={script.visible} onChange={(event) => onToggleVisible(script.id, event.target.checked)} />
                  <span>
                    <strong>{script.name || indicatorTitle(script.source)}</strong>
                    <small>{script.visible ? result?.error || `${result?.provider || "Provider"} · ${result?.barsLoaded || 0} bars` : "Hidden"}</small>
                  </span>
                </label>
                <button type="button" className="ghost-button script-edit-button" onClick={() => onEditScript(script.id)}>Edit</button>
              </div>
            );
          })}
          {loading ? <div className="field-hint">Loading script data…</div> : null}
          {error ? <div className="field-hint error">{error}</div> : null}
        </div>
      ) : null}
    </section>
  );
}

export function ScriptEditorModal({ script, validationError, onTest, onSave, onClose }) {
  const [draft, setDraft] = useState(script);
  useEffect(() => setDraft(script), [script]);
  if (!script || !draft) return null;
  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label="Edit script">
      <div className="modal-card script-modal-card">
        <div className="panel-header">
          <div><h2>Edit Script</h2><p>Supports the Pine-style subset needed for request.security breadth indicators.</p></div>
          <button type="button" className="ghost-button" onClick={onClose}>Close</button>
        </div>
        <label className="script-name-field">Name
          <input type="text" className="settings-input" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </label>
        <textarea className="script-editor" spellCheck="false" value={draft.source} onChange={(event) => setDraft({ ...draft, source: event.target.value })} />
        {validationError ? <div className={`field-hint${validationError.startsWith("OK:") ? "" : " error"}`}>{validationError}</div> : null}
        <div className="button-row script-modal-actions">
          <button type="button" className="ghost-button" onClick={() => onTest(draft)}>Test</button>
          <button type="button" className="primary-button" onClick={() => onSave(draft)}>Save</button>
        </div>
      </div>
    </div>
  );
}
