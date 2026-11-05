import {
  Activity,
  ArrowDownRight,
  ArrowRight,
  Check,
  CircleHelp,
  Eye,
  Fingerprint,
  Mail,
  Smartphone,
  ShieldCheck,
  TriangleAlert,
} from 'lucide-react';
import { useState } from 'react';
import { Link } from 'wouter';

export default function CompanyDashboard() {
  const [findingType, setFindingType] = useState<'all' | 'behavior' | 'payment'>('all');
  const [alertOnly, setAlertOnly] = useState(false);
  const [emailAlertsEnabled, setEmailAlertsEnabled] = useState(true);
  const [showEmailPreview, setShowEmailPreview] = useState(false);
  const [smsAlertsEnabled, setSmsAlertsEnabled] = useState(true);
  const [showSmsPreview, setShowSmsPreview] = useState(false);
  const showBehavior = (findingType === 'all' || findingType === 'behavior') && !alertOnly;
  const showPayment = (findingType === 'all' || findingType === 'payment');
  const visibleCount = Number(showBehavior) + Number(showPayment);

  return (
    <main className="page-shell dashboard-page" data-testid="page-company-dashboard">
      <header className="topbar dashboard-topbar">
        <Link href="/" className="brand" aria-label="FraudGuard overview" data-testid="link-dashboard-brand">
          <span className="brand-mark"><ShieldCheck size={17} strokeWidth={1.8} /></span>
          <span>FraudGuard <small>for Bifrost</small></span>
        </Link>
        <nav className="nav-links dashboard-nav" aria-label="Main navigation">
          <Link href="/" data-testid="link-dashboard-overview">Overview</Link>
          <Link href="/results" data-testid="link-dashboard-results">Demo results</Link>
          <Link href="/for-companies" data-testid="link-dashboard-companies">For companies</Link>
          <Link href="/company-dashboard" aria-current="page" data-testid="link-dashboard-current">Dashboard preview</Link>
        </nav>
      </header>

      <div className="dashboard-wrap">
        <section className="dashboard-intro">
          <div>
            <div className="section-kicker">Operator view / synthetic preview</div>
            <h1>Behavior, in<br /><em>your review path.</em></h1>
            <p>A fixed illustration of how an organization could review findings from its own AI gateway traffic. This is not a live dashboard.</p>
          </div>
          <aside className="dashboard-org">
            <span className="dashboard-org-label">ILLUSTRATIVE ORGANIZATION</span>
            <strong>AshaRatna Tech</strong>
            <span className="dashboard-org-note">Illustrative demo label · not a subscriber</span>
          </aside>
        </section>

        <section className="dashboard-disclaimer" role="note" data-testid="notice-synthetic-company">
          <TriangleAlert size={19} />
          <div>
            <strong>AshaRatna Tech is an illustrative demo label only—not a real subscriber account or an actual partnership.</strong>
            <p>This fixed demo contains no AshaRatna Tech data. Display filters only change which synthetic findings are visible; no live traffic is filtered or blocked, and synthetic scores remain unchanged. There is no live feed and no live plugin events.</p>
          </div>
          <span className="dashboard-fixed-tag">FIXED DEMO</span>
        </section>

        <section className="dashboard-subscription" aria-label="Simulated subscription" data-testid="card-demo-subscription">
          <div className="subscription-mark"><Check size={16} /></div>
          <div className="subscription-copy">
            <span className="dashboard-index">SUBSCRIBER EXPERIENCE · SIMULATED</span>
            <h2>Display filters + alert preferences</h2>
            <p>This prototype illustrates a company plan. Nothing is actually subscribed, billed, or connected.</p>
          </div>
          <div className="subscription-state" data-testid="status-demo-subscription">
            <span>ACTIVE IN SIMULATION</span>
            <strong>Not billed · not actually subscribed</strong>
          </div>
        </section>

        <section className="dashboard-toolbar" aria-label="Preview status">
          <div className="dashboard-toolbar-title"><span className="dashboard-live-dot" /> Review queue <small>synthetic sample findings</small></div>
          <div className="dashboard-toolbar-state"><Eye size={14} /> Observe-only <i /> Requests pass through unchanged</div>
        </section>

        <section className="dashboard-controls" aria-label="Synthetic finding display controls">
          <div className="finding-filter">
            <div>
              <span className="dashboard-index">DISPLAY FILTERS</span>
              <p>Choose which fixed findings to show. These controls do not filter gateway requests.</p>
            </div>
            <div className="filter-buttons" role="group" aria-label="Finding type">
              <button type="button" className={findingType === 'all' ? 'is-selected' : ''} aria-pressed={findingType === 'all'} onClick={() => setFindingType('all')} data-testid="filter-findings-all">All findings</button>
              <button type="button" className={findingType === 'behavior' ? 'is-selected' : ''} aria-pressed={findingType === 'behavior'} onClick={() => setFindingType('behavior')} data-testid="filter-findings-behavior">Request behavior</button>
              <button type="button" className={findingType === 'payment' ? 'is-selected' : ''} aria-pressed={findingType === 'payment'} onClick={() => setFindingType('payment')} data-testid="filter-findings-payment">Trusted payment</button>
            </div>
            <label className="alert-filter">
              <input type="checkbox" checked={alertOnly} onChange={(event) => setAlertOnly(event.target.checked)} data-testid="toggle-alert-only" />
              <span className="filter-switch" aria-hidden="true" />
              <span>Alert-only</span>
            </label>
          </div>
          <div className="dashboard-visible-count" data-testid="status-visible-findings" aria-live="polite">
            <strong>{visibleCount}</strong> of 2 synthetic findings shown
          </div>
        </section>

        <section className="dashboard-findings" aria-label="Synthetic findings">
          {showBehavior && <article className="dashboard-finding behavior-finding" data-testid="finding-dashboard-behavior">
            <header className="dashboard-finding-head">
              <div>
                <span className="dashboard-index">FINDING 01 · REQUEST BEHAVIOR</span>
                <h2>Rapid request sequence</h2>
              </div>
              <span className="dashboard-status status-review"><i /> Below alert threshold</span>
            </header>
            <div className="dashboard-score-row">
              <div className="dashboard-score"><span>Score</span><strong data-testid="score-dashboard-behavior">30<small> / 100</small></strong></div>
              <div className="dashboard-threshold">
                <div><span>Alert threshold</span><strong data-testid="threshold-dashboard-behavior">60</strong></div>
                <div className="dashboard-meter" aria-label="Score 30 of 100, threshold 60"><i className="meter-threshold" /><i className="meter-score" /></div>
                <small><span>0</span><span>30 score</span><span>60 threshold</span><span>100</span></small>
              </div>
              <div className="dashboard-alert-state"><span>No alert</span><strong data-testid="status-dashboard-behavior">alert = false</strong></div>
            </div>
            <div className="dashboard-finding-details">
              <div className="dashboard-signal-block">
                <div className="dashboard-label">Signal contribution</div>
                <div className="dashboard-signal" data-testid="signal-dashboard-rapid-sequence">
                  <Activity size={15} />
                  <div><strong>rapid_sequence</strong><small>Request arrived much sooner than the recent interval pattern.</small></div>
                  <b>+30</b>
                </div>
                <div className="dashboard-signal-total"><span>Total score</span><strong>30 / 100</strong></div>
              </div>
              <div className="dashboard-sequence-block">
                <div className="dashboard-label">Synthetic request cadence</div>
                <p>Five warmup requests spaced <strong>30 seconds apart</strong>, then request six <strong>1 second</strong> after the prior request.</p>
                <div className="dashboard-timeline" data-testid="timeline-dashboard-behavior">
                  <div className="timeline-warmup"><span>Warmup · requests 1–5</span><div><i /><i /><i /><i /><i /></div><b>5 × 30s apart</b></div>
                  <div className="timeline-trigger"><span>Request 6</span><i /><b>1s later</b></div>
                </div>
              </div>
            </div>
            <footer className="dashboard-card-foot"><span>DISPOSITION</span><p><Check size={13} /> Observe-only / pass-through. No request action is taken.</p></footer>
          </article>}

          {showPayment && <article className="dashboard-finding payment-finding" data-testid="finding-dashboard-payment">
            <header className="dashboard-finding-head">
              <div>
                <span className="dashboard-index">FINDING 02 · TRUSTED HOST INPUT</span>
                <h2>New payment location + velocity</h2>
              </div>
              <span className="dashboard-status status-alert"><i /> Alert threshold reached</span>
            </header>
            <div className="dashboard-trusted-note" data-testid="notice-dashboard-payment-input">
              <ShieldCheck size={16} />
              <p><strong>Separate trusted host inputs</strong><span>Payment signals are supplied by a host integration. They are not automatically observed by Bifrost’s request hook.</span></p>
            </div>
            <div className="dashboard-score-row">
              <div className="dashboard-score"><span>Score</span><strong data-testid="score-dashboard-payment">95<small> / 100</small></strong></div>
              <div className="dashboard-threshold">
                <div><span>Alert threshold</span><strong data-testid="threshold-dashboard-payment">60</strong></div>
                <div className="dashboard-meter payment-meter" aria-label="Score 95 of 100, threshold 60"><i className="meter-threshold" /><i className="meter-score" /></div>
                <small><span>0</span><span>60 threshold</span><span>95 score</span><span>100</span></small>
              </div>
              <div className="dashboard-alert-state payment-alert-state"><span>Alert</span><strong data-testid="status-dashboard-payment">alert = true</strong></div>
            </div>
            <div className="dashboard-finding-details">
              <div className="dashboard-signal-block">
                <div className="dashboard-label">Signal contributions</div>
                <div className="dashboard-signal" data-testid="signal-dashboard-payment-location">
                  <Activity size={15} />
                  <div><strong>payment_new_location</strong><small>New trusted coarse country after two warmup events.</small></div>
                  <b>+35</b>
                </div>
                <div className="dashboard-signal" data-testid="signal-dashboard-payment-velocity">
                  <Activity size={15} />
                  <div><strong>payment_velocity</strong><small>Third event exceeds the configured maximum of two per window.</small></div>
                  <b>+60</b>
                </div>
                <div className="dashboard-signal-total"><span>Combined, capped score</span><strong>95 / 100</strong></div>
              </div>
              <div className="dashboard-sequence-block">
                <div className="dashboard-label">Synthetic host-supplied country sequence</div>
                <p>Three events spaced at <strong>30-second intervals</strong>; two warmup events, then a new country and a count above the configured maximum.</p>
                <div className="dashboard-payment-events" data-testid="timeline-dashboard-payment">
                  <div><span>01</span><strong>US</strong><small>Warmup</small><b>0s</b></div>
                  <div><span>02</span><strong>US</strong><small>Warmup</small><b>30s</b></div>
                  <div className="payment-trigger"><span>03</span><strong>CA</strong><small>New country · event 3</small><b>60s</b></div>
                </div>
                <div className="dashboard-payment-limit"><span>Configured maximum</span><strong>2 payments per window</strong></div>
              </div>
            </div>
            <footer className="dashboard-card-foot"><span>DISPOSITION</span><p><Eye size={13} /> Review signal only. No payment is authorized or rejected.</p></footer>
          </article>}

          {visibleCount === 0 && (
            <div className="dashboard-empty-state" data-testid="empty-findings">
              <Eye size={18} />
              <strong>No findings match these display filters.</strong>
              <p>Try showing all findings or turning off Alert-only. No gateway requests are filtered or changed.</p>
            </div>
          )}
        </section>

        <section className="dashboard-alerts" aria-label="Email and SMS alert preferences">
          <div className="alerts-preference">
            <div className="alerts-icon"><Mail size={17} /></div>
            <div className="alerts-copy">
              <span className="dashboard-index">EMAIL ALERTS · LOCAL DEMO STATE</span>
              <h2>Email alert preference</h2>
              <p>Simulated preference only. No email address is collected and no email can be sent.</p>
            </div>
            <label className="email-toggle">
              <input type="checkbox" checked={emailAlertsEnabled} onChange={(event) => {
                setEmailAlertsEnabled(event.target.checked);
                setShowEmailPreview(false);
              }} data-testid="toggle-email-alerts" />
              <span className="filter-switch" aria-hidden="true" />
              <span>{emailAlertsEnabled ? 'Enabled in demo' : 'Disabled in demo'}</span>
            </label>
          </div>
          <div className="email-preview-action">
            <p>Preview the alert for the synthetic payment finding.</p>
            <button type="button" onClick={() => setShowEmailPreview((current) => !current)} disabled={!emailAlertsEnabled} data-testid="button-preview-email">
              <Mail size={14} /> {showEmailPreview ? 'Close email preview' : 'Preview alert email'}
            </button>
          </div>
          <div className="email-preview-live" aria-live="polite" data-testid="status-email-preview">
            {showEmailPreview && emailAlertsEnabled ? (
              <article className="email-preview" data-testid="preview-synthetic-email">
                <div className="email-preview-head"><span>SYNTHETIC EMAIL PREVIEW</span><strong>NOT SENT</strong></div>
                <dl>
                  <div><dt>To</dt><dd>Not configured in this prototype</dd></div>
                  <div><dt>Subject</dt><dd>FraudGuard alert preview: synthetic payment finding</dd></div>
                </dl>
                <div className="email-preview-body">
                  <p>A synthetic payment finding reached its alert threshold.</p>
                  <ul>
                    <li>Score: 95 · threshold: 60 · alert=true</li>
                    <li>Signals: payment_new_location +35; payment_velocity +60</li>
                    <li>Synthetic country sequence: US, US, CA at 30-second intervals</li>
                  </ul>
                  <p>No payment action was taken. Requests remain observe-only and pass through unchanged.</p>
                </div>
              </article>
            ) : (
              <span>{emailAlertsEnabled ? 'Email preview is closed. Nothing has been sent.' : 'Email alerts are disabled in this local demo; preview is unavailable.'}</span>
            )}
          </div>

          <div className="alerts-preference sms-preference">
            <div className="alerts-icon"><Smartphone size={17} /></div>
            <div className="alerts-copy">
              <span className="dashboard-index">SMS ALERTS · LOCAL DEMO STATE</span>
              <h2>SMS alert preference</h2>
              <p>Simulated preference only. No phone number is collected and no SMS can be sent.</p>
            </div>
            <label className="email-toggle sms-toggle">
              <input type="checkbox" checked={smsAlertsEnabled} onChange={(event) => {
                setSmsAlertsEnabled(event.target.checked);
                setShowSmsPreview(false);
              }} data-testid="toggle-sms-alerts" />
              <span className="filter-switch" aria-hidden="true" />
              <span>{smsAlertsEnabled ? 'Enabled in demo' : 'Disabled in demo'}</span>
            </label>
          </div>
          <div className="sms-preview-action">
            <p>Preview a short SMS for the synthetic payment finding.</p>
            <button type="button" onClick={() => setShowSmsPreview((current) => !current)} disabled={!smsAlertsEnabled} data-testid="button-preview-sms">
              <Smartphone size={14} /> {showSmsPreview ? 'Close SMS preview' : 'Preview SMS alert'}
            </button>
          </div>
          <div className="sms-preview-live" aria-live="polite" data-testid="status-sms-preview">
            {showSmsPreview && smsAlertsEnabled ? (
              <article className="email-preview sms-preview" data-testid="preview-synthetic-sms">
                <div className="email-preview-head"><span>SYNTHETIC SMS PREVIEW</span><strong>NOT SENT</strong></div>
                <dl>
                  <div><dt>To</dt><dd>Not configured in this prototype</dd></div>
                </dl>
                <div className="email-preview-body">
                  <p>FraudGuard demo: payment finding alert. Score 95 (threshold 60); new location + velocity signals. Review only—no payment action taken.</p>
                </div>
              </article>
            ) : (
              <span>{smsAlertsEnabled ? 'SMS preview is closed. Nothing has been sent.' : 'SMS alerts are disabled in this local demo; preview is unavailable.'}</span>
            )}
          </div>
          <p className="alerts-prototype-note">This prototype cannot send email or SMS. Both previews are synthetic and marked NOT SENT.</p>
        </section>

        <section className="dashboard-path" data-testid="content-dashboard-integration-path">
          <div className="dashboard-path-heading">
            <div className="section-kicker">The last step, accurately</div>
            <h2>A callback could<br /><em>feed your sink.</em></h2>
          </div>
          <div className="dashboard-path-copy">
            <p>In a real host integration, the plugin’s sanitized <code>OnFinding</code> callback could feed a company-owned sink or dashboard. Findings omit raw keys and prompts.</p>
            <div className="dashboard-path-flow" aria-label="Proposed finding flow">
              <span>Bifrost plugin</span><ArrowRight size={14} /><span>Sanitized finding</span><ArrowRight size={14} /><span>Company-owned sink</span>
            </div>
            <p className="dashboard-path-caveat"><CircleHelp size={14} /> This page is not connected to that callback. No live plugin events reach this UI.</p>
          </div>
        </section>

        <section className="dashboard-limits" data-testid="notice-dashboard-prototype-limits">
          <div className="dashboard-limit-icon"><Fingerprint size={18} /></div>
          <div>
            <span className="dashboard-index">PROTOTYPE SCOPE</span>
            <h2>Bifrost-only, experimental.</h2>
            <p>FraudGuard is an experimental separate Go module supplied through <code>core.Init</code>; it is not built into upstream Bifrost. This company view is illustrative only. Other gateways require their own adapter and validation.</p>
          </div>
          <Link href="/for-companies" className="dashboard-more-link" data-testid="link-dashboard-scope">Company context <ArrowDownRight size={14} /></Link>
        </section>

        <footer className="dashboard-footer">
          <span>FRAUDGUARD · FIXED SYNTHETIC PREVIEW</span>
          <div>
            <Link href="/" data-testid="link-dashboard-footer-overview">Overview</Link>
            <Link href="/results" data-testid="link-dashboard-footer-results">Demo results</Link>
            <Link href="/for-companies" data-testid="link-dashboard-footer-companies">For companies</Link>
          </div>
          <span>NO LIVE FEED · NO CUSTOMER DATA</span>
        </footer>
      </div>
    </main>
  );
}
