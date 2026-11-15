import { Activity, ArrowRight, Cpu, ShieldCheck, UserRound } from 'lucide-react';

function PulseLink({ label, plain = false }: { label: string; plain?: boolean }) {
  return (
    <div className={`heartbeat-link${plain ? ' heartbeat-link-plain' : ''}`} aria-label={label}>
      <span>{label}</span>
      <svg className="heartbeat-wave-horizontal" viewBox="0 0 150 44" aria-hidden="true">
        <path className="heartbeat-wave-track" d="M2 22h146" />
        {plain ? (
          <path className="heartbeat-wave-line" d="M2 22h136l10-7" />
        ) : (
          <path className="heartbeat-wave-line heartbeat-wave-pulse" d="M2 22h38l9 0 9-14 12 29 12-22 9 7h59" />
        )}
      </svg>
      <svg className="heartbeat-wave-vertical" viewBox="0 0 36 64" aria-hidden="true">
        <path className="heartbeat-wave-track" d="M18 1v62" />
        {plain ? (
          <path className="heartbeat-wave-line" d="M18 1v62" />
        ) : (
          <path className="heartbeat-wave-line heartbeat-wave-pulse" d="M18 1v15l-9 8 18 9-9 10v20" />
        )}
      </svg>
    </div>
  );
}

export function BehaviorHeartbeat() {
  return (
    <section className="section heartbeat-section" id="behavior-heartbeat" aria-labelledby="heartbeat-title">
      <div className="heartbeat-heading">
        <div className="section-heading">
          <div className="section-kicker">A behavior pulse · illustrative</div>
          <h2 id="heartbeat-title">People ask.<br /><em>FraudGuard observes.</em></h2>
          <p>
            See where the plugin sits in the request path: it observes behavior inside Bifrost,
            while the request continues to the model as usual.
          </p>
        </div>
        <div className="heartbeat-boundary">
          <Activity size={17} aria-hidden="true" />
          <div>
            <strong>Behavior signal, not a health reading</strong>
            <span>The heartbeat line is a visual metaphor, not biometric data.</span>
          </div>
        </div>
      </div>

      <div className="heartbeat-card">
        <div className="heartbeat-card-head">
          <span><i /> REQUEST FLOW · OBSERVE-ONLY</span>
          <b>SYNTHETIC DEMO</b>
        </div>

        <div className="heartbeat-flow">
          <article className="heartbeat-node heartbeat-person">
            <span className="heartbeat-node-icon"><UserRound size={19} aria-hidden="true" /></span>
            <strong>Person / app</strong>
            <small>AI request starts</small>
          </article>

          <PulseLink label="REQUEST" />

          <article className="heartbeat-gateway">
            <div className="heartbeat-gateway-head">
              <span>AI GATEWAY</span>
              <span className="heartbeat-gateway-state">BIFROST</span>
            </div>
            <div className="heartbeat-plugin">
              <span className="heartbeat-plugin-icon"><ShieldCheck size={19} aria-hidden="true" /></span>
              <span><strong>FraudGuard</strong><small>plugin hook · observe-only</small></span>
              <span className="heartbeat-plugin-live">READS SIGNALS</span>
            </div>
            <div className="heartbeat-gateway-foot">Request passes through unchanged</div>
          </article>

          <PulseLink label="UNCHANGED REQUEST" plain />

          <article className="heartbeat-node heartbeat-model">
            <span className="heartbeat-node-icon"><Cpu size={19} aria-hidden="true" /></span>
            <strong>Model provider</strong>
            <small>Bifrost routes normally</small>
          </article>
        </div>

        <div className="heartbeat-output-label">
          <span className="heartbeat-output-rule" aria-hidden="true" />
          <span>Payload-free finding · request path is not changed</span>
          <ArrowRight size={14} aria-hidden="true" />
        </div>

        <div className="heartbeat-analytics">
          <div className="heartbeat-analytics-head">
            <div>
              <span className="heartbeat-analytics-kicker">WHAT THE PLUGIN REPORTS</span>
              <h3>One small change in the activity pattern.</h3>
            </div>
            <span className="heartbeat-synthetic-label">ACTUAL GO ENGINE · SYNTHETIC INPUTS</span>
          </div>

          <div className="heartbeat-metrics" aria-label="Synthetic demo analytics">
            <div><span>Requests evaluated</span><strong>6</strong><small>one synthetic run</small></div>
            <div><span>Behavior profiles</span><strong>1</strong><small>in this process</small></div>
            <div><span>Findings</span><strong>1</strong><small>below alert threshold</small></div>
            <div><span>Requests blocked</span><strong>0</strong><small>observe-only</small></div>
          </div>

          <div className="heartbeat-reading">
            <div className="heartbeat-risk">
              <div className="heartbeat-risk-label"><span>Risk score</span><strong>30 <small>/ 100</small></strong></div>
              <div className="heartbeat-risk-track" role="img" aria-label="Risk score 30 out of 100; alert threshold 60">
                <span />
                <i />
              </div>
              <div className="heartbeat-risk-foot"><span>0</span><span>Alert threshold 60</span><span>100</span></div>
            </div>
            <div className="heartbeat-signal">
              <span className="heartbeat-signal-icon"><Activity size={16} aria-hidden="true" /></span>
              <div><small>Named signal</small><strong>rapid_sequence <b>+30</b></strong></div>
              <span className="heartbeat-signal-outcome">No alert</span>
            </div>
          </div>

          <p className="heartbeat-footnote">
            This snapshot uses the checked-in Go demo output, not company traffic. The person icon
            marks a request source; no personal identity is shown or collected here.
          </p>
        </div>
      </div>
    </section>
  );
}
