export default function PlanEditor({
  value,
  onChange,
  validation,
  onValidate,
  onLoad,
  onSave,
  validating
}) {
  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <h2>Strategy Plan</h2>
          <p>YAML first, JSON accepted. Validation checks rule structure before the run.</p>
        </div>
        <div className="button-row">
          <button type="button" className="ghost-button" onClick={onLoad}>
            Load
          </button>
          <button type="button" className="ghost-button" onClick={onSave}>
            Save
          </button>
          <button type="button" className="primary-button" onClick={onValidate} disabled={validating}>
            {validating ? "Validating..." : "Validate"}
          </button>
        </div>
      </div>
      <textarea
        className="plan-editor"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        spellCheck={false}
      />
      {validation ? (
        <div className={`validation-box ${validation.valid ? "valid" : "invalid"}`}>
          <strong>{validation.valid ? "Plan is valid" : "Plan needs fixes"}</strong>
          {validation.errors?.length ? <div>{validation.errors.join(" | ")}</div> : null}
          {validation.warnings?.length ? <div>{validation.warnings.join(" | ")}</div> : null}
        </div>
      ) : null}
    </section>
  );
}
