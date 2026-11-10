import { type CSSProperties, type ReactNode, useState } from 'react';
import {
  type FraudGuardDemoCase,
  useGetFraudGuardDemo,
} from '@workspace/api-client-react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ErrorBoundary } from '@/components/error-boundary';
import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import NotFound from '@/pages/not-found';
import ForCompanies from '@/pages/for-companies';
import CompanyDashboard from '@/pages/company-dashboard';
import {
  Activity,
  ArrowDownRight,
  ArrowRight,
  Check,
  CircleHelp,
  Copy,
  Eye,
  Fingerprint,
  KeyRound,
  ShieldCheck,
  TerminalSquare,
  TriangleAlert,
} from 'lucide-react';
import {
  Route,
  Switch,
  Link,
  useLocation,
  Router as WouterRouter,
} from 'wouter';

const queryClient = new QueryClient();
const verifyCommands = [
  'cd plugins/fraudguard',
  'set -e',
  'GOSUMDB=sum.golang.org go test -count=1 ./...',
  'GOSUMDB=sum.golang.org go test -race -count=1 ./...',
  'GOSUMDB=sum.golang.org go vet ./...',
  'GOSUMDB=sum.golang.org go run ./cmd/demo',
].join('\n');
const verifiedOutput = `ok   github.com/maximhq/bifrost/plugins/fraudguard  0.240s
ok   github.com/maximhq/bifrost/plugins/fraudguard/cmd/demo  0.111s

ok   github.com/maximhq/bifrost/plugins/fraudguard  1.771s
ok   github.com/maximhq/bifrost/plugins/fraudguard/cmd/demo  1.130s

go vet ./...: PASS (no output)

observe-only finding: score=30 alert=false signals=[rapid_sequence]
metrics: {Profiles:1 Cohorts:0 PaymentSubjects:0 Requests:6 PaymentEvents:0 Findings:1 Alerts:0 ExemptRequests:0 AIReviewsQueued:0 AIReviewsDropped:0 AIReviewsCompleted:0 AIReviewsFailed:0 FindingCallbacksQueued:0 FindingCallbacksDropped:0 FindingCallbacksCompleted:0 FindingCallbacksFailed:0}
synthetic payment finding: score=95 alert=true signals=[payment_new_location payment_velocity]
synthetic key-status finding: score=100 alert=true signals=[key_expired key_exposed key_revoked]
synthetic multi-turn context finding: score=100 alert=true signals=[context_data_exfiltration context_progressive_jailbreak context_schema_manipulation]`;

function Home() {
  const [commandsCopied, setCommandsCopied] = useState(false);
  const demoQuery = useGetFraudGuardDemo({
    query: { enabled: false, retry: false },
  });

  const copyVerifyCommands = async () => {
    try {
      await navigator.clipboard.writeText(verifyCommands);
      setCommandsCopied(true);
    } catch {
      setCommandsCopied(false);
    }
  };

  return (
    <main className="page-shell">
      <header className="topbar">
        <a href="#top" className="brand" aria-label="FraudGuard home" data-testid="link-home">
          <span className="brand-mark"><ShieldCheck size={17} strokeWidth={1.8} /></span>
          <span>FraudGuard <small>for Bifrost</small></span>
        </a>
        <nav className="nav-links" aria-label="Main navigation">
          <Link href="/for-companies" className="results-nav-link" data-testid="link-home-companies">For companies</Link>
          <Link href="/company-dashboard" className="results-nav-link" data-testid="link-home-dashboard">Dashboard preview</Link>
          <Link href="/results" className="results-nav-link" data-testid="link-results">Demo results <ArrowRight size={13} /></Link>
          <a href="#verification" data-testid="link-verification">Test run</a>
          <a href="#repository" data-testid="link-repository">GitHub</a>
          <a href="#signals" data-testid="link-signals">Signals</a>
          <a href="#privacy" data-testid="link-privacy">Data boundaries</a>
          <a className="nav-cta" href="https://github.com/sham-cyber/Plugin" target="_blank" rel="noreferrer" data-testid="link-demo">Open GitHub <ArrowDownRight size={14} /></a>
        </nav>
      </header>

      <section className="hero" id="top">
        <div>
          <div className="eyebrow">Production-oriented module · Go plugin · Observe-only</div>
          <h1>See the shift<br />before it <em>spreads.</em></h1>
          <p className="hero-copy">
            FraudGuard is a tested, production-oriented Go plugin module for Bifrost. Run its tests and command-line demo below to see real engine output from synthetic events; findings never change request handling.
          </p>
          <div className="hero-actions">
            <a className="button-primary" href="#verification" data-testid="link-hero-demo">See verified run <ArrowRight size={15} /></a>
            <a className="button-secondary" href="#repository" data-testid="link-how-it-works">What’s in GitHub <ArrowDownRight size={15} /></a>
            <a className="button-secondary" href="#business-model" data-testid="link-hero-business-model">Free and paid paths <ArrowDownRight size={15} /></a>
          </div>
          <p className="boundary-note">Separate companion module—not built into upstream Bifrost. Requests are never blocked. Prompt checks are opt-in and transient; provider keys are not used or retained.</p>
        </div>

        <div className="terminal-wrap" aria-label="Captured result from the real Go demo using synthetic events">
          <div className="terminal">
            <div className="terminal-top">
              <span className="terminal-lights"><i /><i /><i /></span>
              <span>FRAUDGUARD / GO DEMO</span>
              <span>OBSERVE</span>
            </div>
            <div className="terminal-main">
              <div>
                <div className="score-label">Captured Go demo score</div>
                <div className="score-value">30<span> / 100</span></div>
                <div className="score-caption">alert=false · threshold 60</div>
              </div>
              <div className="terminal-result"><span>REQUEST ACTION</span><strong>NONE</strong><small>observe-only</small></div>
            </div>
            <div className="terminal-events">
              <div className="event-row"><span>rapid_sequence</span><strong>+30</strong><span>01s gap</span></div>
              <div className="event-row"><span>alerts</span><strong>0</strong><span>below threshold</span></div>
            </div>
            <div className="synthetic-stamp"><span><i className="stamp-dot" />ACTUAL CLI OUTPUT · SYNTHETIC INPUTS</span><span>NO IDENTIFIERS</span></div>
          </div>
        </div>
      </section>

      <section className="section verification-section" id="verification">
        <div className="verification-heading">
          <div className="section-kicker">Real commands · actual Go module</div>
          <h2>Run it.<br /><em>See the result.</em></h2>
          <p>These commands were run from this project’s <code>plugins/fraudguard</code> folder. The output beside them is captured from that run—not a mock browser result.</p>
        </div>
        <div className="verification-console">
          <div className="verification-console-head">
            <span><i /> Replit Shell · FraudGuard</span>
            <button type="button" onClick={copyVerifyCommands} className="copy-commands" data-testid="button-copy-commands">
              {commandsCopied ? <Check size={13} /> : <Copy size={13} />}
              {commandsCopied ? 'Copied' : 'Copy commands'}
            </button>
          </div>
          <div className="verification-columns">
            <div className="verification-pane">
              <div className="verification-label">Commands</div>
              <pre><code>{verifyCommands}</code></pre>
            </div>
            <div className="verification-pane">
              <div className="verification-label">Captured execution · all checks passed</div>
              <pre><code>{verifiedOutput}</code></pre>
            </div>
          </div>
          <p className="verification-note">This page displays the captured run; it does not execute shell commands. To run it live for your video, open Replit Shell at the project root, paste the copied commands, and record the output.</p>
        </div>
      </section>

      <section className="section repository-section" id="repository">
        <div className="repository-heading">
          <div className="section-kicker">The Go module</div>
          <h2>What’s in <em>GitHub.</em></h2>
          <p>The GitHub repository contains the Go plugin module. This page is a separate explainer and synthetic demo, not the plugin runtime.</p>
          <a className="button-primary" href="https://github.com/sham-cyber/Plugin" target="_blank" rel="noreferrer" data-testid="link-github-repository">
            Open sham-cyber/Plugin <ArrowRight size={14} />
          </a>
        </div>
        <div className="repository-panel">
          <div className="repository-status"><span className="repository-dot" /> Go plugin is in GitHub <span className="repository-license">Apache-2.0</span></div>
          <ul>
            <li><code>plugins/fraudguard/engine.go</code><span>Behavior scoring and bounded profiles</span></li>
            <li><code>plugins/fraudguard/plugin.go</code><span>Bifrost plugin hooks</span></li>
            <li><code>plugins/fraudguard/*_test.go</code><span>Unit, privacy, and concurrency tests</span></li>
            <li><code>plugins/fraudguard/cmd/demo</code><span>Offline synthetic findings demo</span></li>
            <li><span><code>README.md</code> · <code>VERIFICATION_REPORT.md</code></span><span>Setup, evidence, limits, and license</span></li>
          </ul>
          <a className="repository-commit" href="https://github.com/sham-cyber/Plugin/commit/224d4ebc165e0db95ad9c88e3cb70ea58edca818" target="_blank" rel="noreferrer">
            GitHub feature commit: 224d4eb · optional payment and key-status signals
          </a>
          <p className="repository-local-note">The Bifrost explainer page is currently in this Replit workspace and has not yet been pushed to GitHub. The plugin files above are already on the repository’s main branch.</p>
        </div>
      </section>

      <section className="section intro-band">
        <div className="section-heading">
          <div className="section-kicker">The gap</div>
          <h2>A request can be<br />within limits—and<br /><em>still be different.</em></h2>
        </div>
        <div>
          <p className="intro-quote">
            “A familiar key suddenly explores unfamiliar models, accelerates, or changes its usage shape.”
            <small>Behavior change is a clue, not a verdict.</small>
          </p>
          <p className="section-heading" style={{ marginTop: 24, color: '#667578', fontSize: 13, lineHeight: 1.75 }}>
            Rate limits answer how much traffic is allowed. FraudGuard adds a complementary signal: does this authenticated key look unlike its own recent baseline?
          </p>
        </div>
      </section>

      <section className="section signal-section" id="signals">
        <div className="section-heading">
          <div className="section-kicker">What it watches</div>
          <h2>Small signals.<br /><em>Useful context.</em></h2>
          <p>Simple, inspectable rules compare a key’s recent activity with its own retained history. They are indicators to review—not a model’s judgment of intent.</p>
        </div>
        <div className="signal-grid">
          <div className="signal-list">
            <article className="signal-item"><span className="signal-index">01</span><div><h3>New model or provider</h3><p>A destination not seen for this virtual key in its active rolling window.</p></div><span className="signal-tag">novelty</span></article>
            <article className="signal-item"><span className="signal-index">02</span><div><h3>Unusually rapid sequence</h3><p>Request timing that sharply departs from the key’s own recent interval pattern.</p></div><span className="signal-tag">cadence</span></article>
            <article className="signal-item"><span className="signal-index">03</span><div><h3>Token-volume spike</h3><p>Large input or output usage change, when normalized usage is available.</p></div><span className="signal-tag">volume</span></article>
            <article className="signal-item"><span className="signal-index">04</span><div><h3>Repeated failures</h3><p>A recent run of failed outcomes can add context when the post-hook sees them.</p></div><span className="signal-tag">outcomes</span></article>
            <article className="signal-item"><span className="signal-index">05</span><div><h3>Optional host signals</h3><p>Payment velocity, coarse location, and key-status indicators require explicit trusted host input.</p></div><span className="signal-tag">opt-in only</span></article>
            <article className="signal-item"><span className="signal-index">06</span><div><h3>Optional prompt checks</h3><p>Bounded user-text phrase and size checks are transient, heuristic, and off by default.</p></div><span className="signal-tag">opt-in only</span></article>
          </div>
          <aside className="signal-aside">
            <div className="aside-label"><Activity size={13} /> A score is a starting point</div>
            <h3>One unfamiliar request isn’t proof of abuse.</h3>
            <p>Warmup suppresses anomaly signals while a baseline forms. Legitimate migrations, batch workloads, retries, and changing token usage can still produce false positives.</p>
            <div className="mini-score"><span>Example threshold · configurable</span><b>60 / 100</b></div>
          </aside>
        </div>
      </section>

      <section className="section flow-section" id="how-it-works">
        <div className="flow-layout">
          <div className="section-heading">
            <div className="section-kicker">Under the hook</div>
            <h2>Observe.<br />Compare.<br /><em>Report.</em></h2>
            <p>The companion plugin uses Bifrost’s authenticated internal virtual-key context, then emits structured, payload-free findings for your own review path.</p>
          </div>
          <div className="flow-callout">
            <strong>Pass-through by design.</strong> FraudGuard’s hooks do not block, short-circuit, rewrite, or authorize a request. A score is not an access-control decision.
          </div>
        </div>
        <div className="flow-line">
          <article className="flow-step"><b>01 / CONTEXT</b><h3>Resolved identity</h3><p>Profile only when the authenticated internal virtual-key ID is present.</p></article>
          <article className="flow-step"><b>02 / HISTORY</b><h3>Bounded baseline</h3><p>Compare bounded recent behavior within this process and rolling window.</p></article>
          <article className="flow-step"><b>03 / SIGNALS</b><h3>Fixed reasons</h3><p>Findings contain fixed reasons only; prompt text and error details are never emitted.</p></article>
          <article className="flow-step"><b>04 / REVIEW</b><h3>Your sink, your call</h3><p>Forward structured findings to an operator-owned audit or review pipeline.</p></article>
        </div>
      </section>

      <section className="section demo-section" id="demo">
        <div className="demo-intro">
          <div className="section-heading">
            <div className="section-kicker">Actual Go engine · synthetic inputs</div>
            <h2>Run the detector.<br /><em>See its output.</em></h2>
            <p>The page asks the local API to run fixed synthetic scenarios through the FraudGuard Go engine. These are real detector results, not company traffic.</p>
          </div>
        </div>
        <div className="demo-panel">
          <div className="demo-controls">
            <div className="micro">Local Go engine · fixed synthetic events</div>
            <h3>Baseline, then a change.</h3>
            <p>Run request behavior, trusted payment, and key-status examples through the same Go scoring engine used by the plugin.</p>
            <button
              className="demo-run"
              type="button"
              onClick={() => void demoQuery.refetch()}
              disabled={demoQuery.isFetching}
              aria-busy={demoQuery.isFetching}
              data-testid="button-run-demo"
            >
              <TerminalSquare size={15} />
              {demoQuery.isFetching ? 'Running Go engine…' : demoQuery.data ? 'Run again' : 'Run Go detector'}
            </button>
            <small>No customer requests or credentials are sent. The API runs only fixed synthetic cases.</small>
          </div>
          <div className="demo-output" aria-live="polite">
            <div className="output-head">
              <span>Go engine finding stream</span>
              <span className="output-state">
                {demoQuery.isFetching ? 'running' : demoQuery.data ? 'actual engine output' : 'not run'}
              </span>
            </div>
            {demoQuery.isError ? (
              <div className="demo-api-error" role="alert">
                The local Go demo could not run. Check that the API server and Go toolchain are available, then try again.
              </div>
            ) : demoQuery.data ? (
              <div className="finding-list">
                {demoQuery.data.cases.map((demoCase, index) => (
                  <div className="finding" key={demoCase.id} style={{ animationDelay: `${index * 0.08}s` }}>
                    <span className="finding-icon"><Activity size={12} /></span>
                    <div>
                      <strong>{demoCase.title} · {demoCase.finding.score}/100</strong>
                      <span>
                        {demoCase.finding.signals.map((signal) => `${signal.name} +${signal.weight}`).join(' · ') || 'No anomaly signals'}
                      </span>
                    </div>
                    <span className="finding-score">
                      {demoCase.finding.alert ? `alert ≥ ${demoCase.finding.threshold}` : 'observe'}
                    </span>
                  </div>
                ))}
              </div>
            ) : demoQuery.isFetching ? (
              <div className="empty-output" role="status">
                <b><Activity size={16} /></b>
                Running fixed synthetic events through the Go detector…
              </div>
            ) : (
              <div className="empty-output">
                <b><Eye size={16} /></b>
                Nothing has run yet. Start the Go detector to see its actual findings.<br />
                Only fixed synthetic data is used.
              </div>
            )}
          </div>
        </div>
        <div className="demo-disclaimer">
          <CircleHelp size={14} />
          This page calls the local demo API, which executes the actual Go engine with fixed synthetic inputs. It does not connect to a company gateway or AI provider.
        </div>
      </section>

      <section className="section privacy-section" id="privacy">
        <div className="section-heading">
          <div className="section-kicker">Data boundaries</div>
          <h2>Observe the context.<br /><em>Not the secret.</em></h2>
          <p>FraudGuard is designed to compare behavior without turning requests into a content log or handling credentials.</p>
          <div className="credential-box">
            <strong><KeyRound size={13} /> PROVIDER CREDENTIAL BOUNDARY</strong>
            <p>Provider API keys stay configured in Bifrost. This page and plugin must never request, display, store, or log them. The adapter observes authenticated request context and behavior signals—not raw provider key values.</p>
          </div>
        </div>
        <div>
          <div className="privacy-grid">
            <article className="privacy-item"><h3><Fingerprint size={14} /> Internal identity only</h3><p>Uses the resolved internal virtual-key ID; no fallback to raw key headers or credential values.</p></article>
            <article className="privacy-item"><h3><Eye size={14} /> No prompt retention</h3><p>Opt-in checks scan bounded user text transiently; findings omit prompts, responses, payloads, request IDs, and raw error text.</p></article>
            <article className="privacy-item"><h3><ShieldCheck size={14} /> Process-local state</h3><p>Behavior history is bounded and in-memory per process. Restart clears it; replicas do not share baselines.</p></article>
            <article className="privacy-item"><h3><Activity size={14} /> Minimal findings</h3><p>Structured score, threshold, flags, and fixed signal reasons—without key identifiers or destination labels.</p></article>
          </div>
          <div className="credential-box">
            <strong>OPTIONAL SIGNALS ARE NOT DISCOVERED</strong>
            <p>Payment events and credential-status booleans come only from explicit trusted host integrations. The normal Bifrost request hook does not automatically discover payment data or key status.</p>
          </div>
        </div>
      </section>

      <section className="section integration-section" id="integration">
        <div className="section-heading">
          <div className="section-kicker">For gateway engineers</div>
          <h2>A companion module,<br /><em>not a built-in.</em></h2>
          <p>FraudGuard lives as a separate Go module in this project. It is not included in upstream Bifrost, and no upstream acceptance or merge is claimed.</p>
        </div>
        <div className="integration-grid">
          <div className="integration-copy">
            <p>To integrate the module, copy it into a matching Bifrost checkout, add it to that app’s module/workspace, construct it, and register it with <code>core.Init</code>.</p>
            <ol>
              <li>Review the pinned Bifrost core API contract.</li>
              <li>Set warmup, window, profile bounds, and threshold for your environment.</li>
              <li>Forward payload-free findings to an operator-controlled sink.</li>
              <li>Review alert behavior before relying on signals.</li>
            </ol>
            <p>The standard upstream HTTP server loader is not modified by this direct integration path.</p>
          </div>
          <div className="code-card">
            <div className="code-head"><span>GO · REGISTRATION SKETCH</span><span>NO PROVIDER KEY INPUT</span></div>
            <pre><code><span className="code-muted">// Companion module setup (illustrative)</span>{'\n'}plugin, err := fraudguard.New(fraudguard.Config{'{'}{'\n'}  WarmupRequests: 5,{'\n'}  AlertThreshold: 60,{'\n'}  OnFinding: func(f fraudguard.Finding) {'{'}{'\n'}    <span className="code-muted">// Send structured finding to your sink.</span>{'\n'}  {'}'},{'\n'}{'}'}){'\n'}if err != nil {'{'}{'\n'}  return err{'\n'}{'}'}{'\n'}client, err := bifrost.Init(ctx, schemas.BifrostConfig{'{'}{'\n'}  LLMPlugins: []schemas.LLMPlugin{'{'}plugin{'}'},{'\n'}{'}'})</code></pre>
            <div className="code-foot">Example shape only · see companion module docs for configuration and exact registration details</div>
          </div>
        </div>
      </section>

      <section className="section business-model-section" id="business-model" data-testid="section-business-model">
        <div className="business-model-intro">
          <div className="section-heading">
            <div className="section-kicker">Free hackathon · commercial routes proposed</div>
            <h2>Free to try.<br /><em>Built to earn later.</em></h2>
            <p>The hackathon edition stays free. If companies find value, AshaRatna could sell a managed service directly, or Bifrost could distribute it under an agreement. Neither paid route exists today.</p>
          </div>
          <aside className="business-model-revenue" role="note" data-testid="note-revenue-model">
            <span>REVENUE MODEL · NOT A FORECAST</span>
            <strong>Company subscription for ongoing service.</strong>
            <p>Proposed pricing is for managed features and support—not a new charge every time software is deployed. No price, customer count, or profit is promised.</p>
          </aside>
        </div>

        <div className="business-model-grid">
          <article className="business-model-card business-model-free" data-testid="card-business-free">
            <span className="business-model-label">NOW · FREE HACKATHON EDITION</span>
            <h3>Open-source core</h3>
            <p>Apache-2.0 plugin, tests, and synthetic demo. No FraudGuard license key, checkout, or subscription is needed.</p>
            <ul>
              <li><Check size={14} /> Self-hosted Bifrost prototype</li>
              <li><Check size={14} /> Observe-only behavior findings</li>
              <li><Check size={14} /> Free to review and test</li>
            </ul>
            <div className="business-model-foot">Available for the hackathon · no billing</div>
          </article>

          <article className="business-model-card business-model-asha" data-testid="card-business-asharatna">
            <span className="business-model-label">PROPOSED · ASHARATNA DIRECT</span>
            <h3>Sell a managed service</h3>
            <p>AshaRatna Tech could sell a company workspace with hosted reporting, multi-instance visibility, verified alerts, retention, and support.</p>
            <ul>
              <li><Check size={14} /> Monthly or annual company subscription</li>
              <li><Check size={14} /> Price around service level, not redeployment</li>
              <li><Check size={14} /> Validate price with pilot buyers first</li>
            </ul>
            <div className="business-model-foot">Proposal only · service and billing are not built</div>
          </article>

          <article className="business-model-card business-model-bifrost" data-testid="card-business-bifrost">
            <span className="business-model-label">POSSIBLE · WITH BIFROST AGREEMENT</span>
            <h3>Offer through Bifrost</h3>
            <p>If Bifrost approves and agrees to distribute it, the plugin service could be bundled or resold through a negotiated commercial arrangement.</p>
            <ul>
              <li><Check size={14} /> Revenue share or license terms by agreement</li>
              <li><Check size={14} /> Requires technical and commercial approval</li>
              <li><Check size={14} /> No partnership or marketplace listing exists</li>
            </ul>
            <div className="business-model-foot">Possible channel · not a current Bifrost offering</div>
          </article>
        </div>

        <div className="business-model-economics" data-testid="overview-money-and-cost-tracker">
          <section className="business-model-finance">
            <span className="business-model-label">PROJECT MONEY OVERVIEW · NO PROFIT FORECAST</span>
            <h3>Free core. Paid service. No profit claim.</h3>
            <p>The open-source plugin is free to review. AshaRatna’s proposed revenue comes from an optional managed company service; a Bifrost channel would require a separate agreement. One gateway deployment may serve many keys.</p>
            <div className="business-model-formula">
              <div>
                <span>Possible monthly revenue</span>
                <strong>Active subscriptions × agreed price + optional overage</strong>
              </div>
              <div>
                <span>Possible net contribution</span>
                <strong>Revenue − hosting, storage, billing, operations, and support</strong>
              </div>
            </div>
            <p className="business-model-finance-note">No price, customer count, sales forecast, or profit is validated. This is a business model, not a guarantee.</p>
          </section>

          <aside className="business-model-cost" data-testid="overview-cost-tracker-preview">
            <span className="business-model-label">ASHARATNA COST TRACKER · SYNTHETIC</span>
            <strong className="business-model-cost-value">$0.78</strong>
            <p>Example provider cost for four synthetic AI requests using public model rates. No requests were blocked; this is not money saved.</p>
            <Link href="/company-dashboard" className="business-model-cost-link" data-testid="link-overview-cost-tracker">
              Open the cost tracker <ArrowRight size={14} />
            </Link>
          </aside>
        </div>

        <div className="business-model-boundary" data-testid="note-paid-service-boundary">
          <strong>Provider keys and FraudGuard access are different.</strong>
          <p>Companies keep their own model-provider API keys in Bifrost. A future FraudGuard entitlement would be a separate key; this prototype does not issue or verify one. Because the core is Apache-2.0, paid value should come from a managed service, added operations, or support—not a claim that users cannot run the open-source plugin.</p>
        </div>
      </section>

      <section className="section limits-section" id="scope">
        <div className="limits-layout">
          <div className="section-heading">
            <div className="section-kicker">Scope, plainly stated</div>
            <h2>Useful to inspect.<br /><em>Not magic.</em></h2>
            <p>The module has deliberately narrow claims. Validate it in a safe environment; use findings as review context, not as an automated verdict.</p>
          </div>
          <div className="limits-grid">
            <div className="limit"><TriangleAlert size={14} /> No live company dashboard, customer database, external AI service, or cross-process state.</div>
            <div className="limit"><TriangleAlert size={14} /> No automatic blocking, request edits, or authorization decision.</div>
            <div className="limit"><TriangleAlert size={14} /> No automatic payment, location, or key-status discovery.</div>
            <div className="limit"><TriangleAlert size={14} /> No semantic intent detection; phrase checks are shallow, optional indicators.</div>
            <div className="limit"><TriangleAlert size={14} /> Baselines reset on process restart and are not shared between replicas.</div>
            <div className="limit"><TriangleAlert size={14} /> False positives are expected; simple rules are not calibrated fraud models.</div>
            <div className="limit"><TriangleAlert size={14} /> Usage spikes need normalized token data; missing usage means no such signal.</div>
            <div className="limit"><TriangleAlert size={14} /> Some failures are only visible if the plugin reaches its post-hook.</div>
          </div>
        </div>
      </section>

      <section className="last-call">
        <div>
          <div className="section-kicker" style={{ color: '#afc9bc' }}>Start with the boundary</div>
          <h2>Keep requests flowing.<br /><em>Make changes visible.</em></h2>
        </div>
        <div>
          <p>Run the tested Go commands, review what is already in GitHub, then inspect the plugin limits before trying it with Bifrost.</p>
          <a className="button-primary" href="#verification" data-testid="link-final-demo">Review the real Go run <ArrowRight size={14} /></a>
        </div>
      </section>
      <footer className="footer">
        <span>FRAUDGUARD FOR BIFROST · EXPERIMENTAL COMPANION MODULE</span>
        <div className="foot-links">
          <a href="#integration" data-testid="link-footer-integration">Integration path</a>
          <a href="#business-model" data-testid="link-footer-business-model">Free and paid model</a>
          <a href="#privacy" data-testid="link-footer-boundaries">Data boundaries</a>
          <a href="#scope" data-testid="link-footer-limitations">Limits</a>
        </div>
        <span>OBSERVE ONLY · NO CREDENTIAL FORM</span>
      </footer>
    </main>
  );
}

function DemoResultCard({
  demoCase,
  index,
}: {
  demoCase: FraudGuardDemoCase;
  index: number;
}) {
  const { finding } = demoCase;
  const trackLabels = [
    { value: 0, label: '0' },
    { value: finding.score, label: `${finding.score} score` },
    { value: finding.threshold, label: `${finding.threshold} threshold` },
    { value: 100, label: '100' },
  ].sort((left, right) => left.value - right.value);
  const scoreStyle = {
    '--score-progress': `${finding.score}%`,
    '--threshold-position': `${finding.threshold}%`,
    '--score-color': finding.alert ? '#d98561' : '#65a089',
  } as CSSProperties;

  return (
    <article className={`result-card ${demoCase.id}-card`} data-testid={`result-${demoCase.id}`}>
      <div className="result-card-top">
        <div>
          <div className="result-index">
            {String(index + 1).padStart(2, '0')} / {demoCase.id.replaceAll('_', ' ')}
          </div>
          <h2>{demoCase.title}<br /><em>{finding.alert ? 'Review signal' : 'Observe-only'}</em></h2>
        </div>
        <div className={`result-outcome ${finding.alert ? 'payment-outcome' : ''}`}>
          <span className="outcome-dot" />
          <span>{finding.alert ? 'ALERT THRESHOLD MET' : 'OBSERVE ONLY'}</span>
          <strong>{finding.alert ? 'Review finding' : 'Pass-through'}</strong>
        </div>
      </div>

      <div className="score-band dynamic-score-band" style={scoreStyle}>
        <div className="score-block">
          <span className="score-label">Finding score</span>
          <div className="result-score" data-testid={`score-${demoCase.id}`}>
            {finding.score}<span> / 100</span>
          </div>
          <span className="score-explainer">
            {finding.alert ? 'At or above alert threshold' : 'Below alert threshold'}
          </span>
        </div>
        <div className="threshold-block">
          <div className="threshold-copy">
            <span>Alert threshold</span>
            <strong data-testid={`threshold-${demoCase.id}`}>{finding.threshold}</strong>
          </div>
          <div
            className="threshold-track"
            role="img"
            aria-label={`Score ${finding.score} of 100; alert threshold ${finding.threshold}`}
          >
            <i className="threshold-marker" />
            <i className="score-marker" />
          </div>
          <div className="track-labels">
            {trackLabels.map((label, labelIndex) => (
              <span key={`${label.label}-${labelIndex}`}>{label.label}</span>
            ))}
          </div>
        </div>
        <div className={`alert-state ${finding.alert ? 'payment-alert' : ''}`}>
          <span className="alert-led" />
          <strong data-testid={`status-${demoCase.id}-alert`}>
            {finding.alert ? 'Review signal' : 'No alert'}
          </strong>
          <small>alert = {String(finding.alert)}</small>
        </div>
      </div>

      <div className="result-detail-grid">
        <div className="signal-pane">
          <div className="pane-label">Signals returned by the Go engine</div>
          {finding.signals.length > 0 ? finding.signals.map((signal, signalIndex) => (
            <div className="signal-row" key={signal.name} data-testid={`signal-${demoCase.id}-${signal.name}`}>
              <span className="signal-mark">{String(signalIndex + 1).padStart(2, '0')}</span>
              <div><strong>{signal.name}</strong><small>{signal.reason}</small></div>
              <b>+{signal.weight}</b>
            </div>
          )) : (
            <p className="no-signals">No anomaly signals were produced for this synthetic scenario.</p>
          )}
          <div className="signal-total">
            <span>Total score</span><strong>{finding.score} / 100</strong>
          </div>
        </div>
        <div className="explain-pane">
          <div className="pane-label">Synthetic scenario sent to the engine</div>
          <p>{demoCase.description}</p>
          <div className="engine-output-note">
            <ShieldCheck size={14} />
            Score and signal reasons above are returned by the actual FraudGuard Go engine.
          </div>
        </div>
      </div>

      <footer className="result-card-foot">
        <span>DISPOSITION</span>
        <p>Review context only. The request or payment is not blocked, rewritten, authorized, or changed.</p>
      </footer>
    </article>
  );
}

function Results() {
  const demoQuery = useGetFraudGuardDemo({
    query: { enabled: true, retry: false },
  });

  return (
    <main className="page-shell results-page">
      <header className="topbar">
        <Link href="/" className="brand" aria-label="FraudGuard home" data-testid="link-results-home">
          <span className="brand-mark"><ShieldCheck size={17} strokeWidth={1.8} /></span>
          <span>FraudGuard <small>for Bifrost</small></span>
        </Link>
        <nav className="nav-links results-nav" aria-label="Main navigation">
          <Link href="/" data-testid="link-results-back">Overview</Link>
          <Link href="/results" aria-current="page" data-testid="link-results-current">Demo results</Link>
          <Link href="/for-companies" data-testid="link-results-companies">For companies</Link>
          <Link href="/company-dashboard" data-testid="link-results-dashboard">Dashboard preview</Link>
        </nav>
      </header>

      <section className="results-hero">
        <div className="results-kicker">Run the actual Go engine · fixed synthetic inputs</div>
        <div className="results-heading-row">
          <div>
            <h1>Findings, <em>made legible.</em></h1>
            <p>Run three controlled synthetic scenarios through the FraudGuard Go engine. The displayed scores and reasons come from the engine—not from a browser mock or company traffic.</p>
          </div>
          <div className="synthetic-seal" data-testid="status-synthetic-fixed">
            <span className="seal-dot" />
            <div><strong>REAL ENGINE · SYNTHETIC INPUT</strong><small>No customer or provider traffic</small></div>
          </div>
        </div>
        <div className="results-run-controls">
          <button
            className="button-primary results-run-button"
            type="button"
            onClick={() => void demoQuery.refetch()}
            disabled={demoQuery.isFetching}
            aria-busy={demoQuery.isFetching}
            data-testid="button-run-go-engine"
          >
            <TerminalSquare size={14} />
            {demoQuery.isFetching ? 'Running Go engine…' : demoQuery.data ? 'Run again' : 'Run Go detector'}
          </button>
          {demoQuery.data && (
            <span className="results-generated-at" data-testid="status-demo-generated">
              Generated {new Date(demoQuery.data.generated_at).toLocaleTimeString()}
            </span>
          )}
        </div>
        <div className="results-boundary" role="note" data-testid="notice-observe-only">
          <ShieldCheck size={18} />
          <span><strong>Observe-only, by design.</strong> Findings add context for an operator; they do not block, rewrite, authorize, or change requests.</span>
          <span className="boundary-chip">PASS-THROUGH</span>
        </div>
      </section>

      <section className="results-content" aria-label="Findings returned by the Go demo">
        {demoQuery.isError ? (
          <div className="demo-api-error results-api-error" role="alert">
            The local Go demo could not run. Check that the API server and Go toolchain are available, then try again.
          </div>
        ) : demoQuery.isFetching ? (
          <div className="demo-run-state" role="status">
            <Activity size={16} /> Running fixed synthetic scenarios through the Go detector…
          </div>
        ) : demoQuery.data ? (
          demoQuery.data.cases.map((demoCase, index) => (
            <DemoResultCard key={demoCase.id} demoCase={demoCase} index={index} />
          ))
        ) : (
          <div className="demo-run-state">
            Select “Run Go detector” to calculate current findings with the actual Go engine.
          </div>
        )}
      </section>

      <section className="results-footnotes">
        <div className="footnote-heading"><span className="result-index">READ THIS FIRST</span><h2>Engine evidence,<br /><em>not live traffic.</em></h2></div>
        <div className="footnote-list">
          <p><b>01</b><span>Inputs are fixed synthetic events from <code>plugins/fraudguard/cmd/demo/main.go</code>; each run executes the actual Go scoring engine.</span></p>
          <p><b>02</b><span>Payment and key-status cases are synthetic trusted host inputs, not live processor, credential, or Bifrost traffic.</span></p>
          <p><b>03</b><span>No prompts, credentials, raw virtual keys, customer records, or external AI-provider calls are involved.</span></p>
        </div>
      </section>
      <footer className="results-footer">
        <span>FRAUDGUARD FOR BIFROST · OBSERVE-ONLY GO MODULE</span>
        <Link href="/" className="results-footer-link" data-testid="link-back-overview"><ArrowRight size={14} /> Back to overview</Link>
        <span>FIXED SYNTHETIC DATA · OBSERVE ONLY</span>
      </footer>
    </main>
  );
}

function Router() {
  return (
    // Keep a shared shell (sidebar, navbar) outside the boundary so it
    // survives a page crash.
    <RoutedErrorBoundary>
      <Switch>
        <Route path="/" component={Home} />
        <Route path="/results" component={Results} />
        <Route path="/for-companies" component={ForCompanies} />
        <Route path="/company-dashboard" component={CompanyDashboard} />
        <Route component={NotFound} />
      </Switch>
    </RoutedErrorBoundary>
  );
}

function RoutedErrorBoundary({ children }: { children: ReactNode }) {
  const [location] = useLocation();
  return <ErrorBoundary resetKey={location}>{children}</ErrorBoundary>;
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <WouterRouter base={import.meta.env.BASE_URL.replace(/\/$/, '')}>
          <Router />
        </WouterRouter>
        <Toaster />
      </TooltipProvider>
    </QueryClientProvider>
  );
}

export default App;
