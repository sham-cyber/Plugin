import { type ReactNode } from 'react';
import { useState } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ErrorBoundary } from '@/components/error-boundary';
import { Toaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import NotFound from '@/pages/not-found';
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
  RotateCcw,
  ShieldCheck,
  TerminalSquare,
  TriangleAlert,
} from 'lucide-react';
import {
  Route,
  Switch,
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
const verifiedOutput = `ok   github.com/maximhq/bifrost/plugins/fraudguard  0.180s
?    github.com/maximhq/bifrost/plugins/fraudguard/cmd/demo  [no test files]

ok   github.com/maximhq/bifrost/plugins/fraudguard  1.730s
?    github.com/maximhq/bifrost/plugins/fraudguard/cmd/demo  [no test files]

go vet ./...: PASS (no output)

observe-only finding: score=30 alert=false signals=[rapid_sequence]
metrics: {Profiles:1 Cohorts:0 PaymentSubjects:0 Requests:6 PaymentEvents:0 Findings:1 Alerts:0 ExemptRequests:0}
synthetic payment finding: score=95 alert=true signals=[payment_new_location payment_velocity]
synthetic key-status finding: score=100 alert=true signals=[key_expired key_exposed key_revoked]`;

function Home() {
  const [showFindings, setShowFindings] = useState(false);
  const [commandsCopied, setCommandsCopied] = useState(false);

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
          <a href="#verification" data-testid="link-verification">Test run</a>
          <a href="#repository" data-testid="link-repository">GitHub</a>
          <a href="#signals" data-testid="link-signals">Signals</a>
          <a href="#privacy" data-testid="link-privacy">Data boundaries</a>
          <a className="nav-cta" href="https://github.com/sham-cyber/Plugin" target="_blank" rel="noreferrer" data-testid="link-demo">Open GitHub <ArrowDownRight size={14} /></a>
        </nav>
      </header>

      <section className="hero" id="top">
        <div>
          <div className="eyebrow">Hackathon prototype · Go plugin · Observe-only</div>
          <h1>See the shift<br />before it <em>spreads.</em></h1>
          <p className="hero-copy">
            FraudGuard is a tested Go plugin prototype for Bifrost. Run its tests and command-line demo below to see real output; the demo uses synthetic events, and flagged requests pass through unchanged.
          </p>
          <div className="hero-actions">
            <a className="button-primary" href="#verification" data-testid="link-hero-demo">See verified run <ArrowRight size={15} /></a>
            <a className="button-secondary" href="#repository" data-testid="link-how-it-works">What’s in GitHub <ArrowDownRight size={15} /></a>
          </div>
          <p className="boundary-note">Separate prototype module—not built into upstream Bifrost. No request blocking, prompt inspection, or provider key input.</p>
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
          <div className="section-kicker">The hackathon code</div>
          <h2>What’s in <em>GitHub.</em></h2>
          <p>The GitHub repository contains the working Go prototype. This page is a separate explainer, not the plugin itself.</p>
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
            Rate limits answer how much traffic is allowed. FraudGuard prototypes a complementary question: does this authenticated key look unlike its own recent baseline?
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
          <article className="flow-step"><b>03 / SIGNALS</b><h3>Fixed reasons</h3><p>Score named behavioral changes; no prompts or error text enter findings.</p></article>
          <article className="flow-step"><b>04 / REVIEW</b><h3>Your sink, your call</h3><p>Forward structured findings to an operator-owned audit or review pipeline.</p></article>
        </div>
      </section>

      <section className="section demo-section" id="demo">
        <div className="demo-intro">
          <div className="section-heading">
            <div className="section-kicker">Browser-only illustration</div>
            <h2>Example signals,<br /><em>not a test run.</em></h2>
            <p>This small interaction is only a visual illustration. For real detector output, use the Shell commands above and the Go demo.</p>
          </div>
        </div>
        <div className="demo-panel">
          <div className="demo-controls">
            <div className="micro">Local-only · Synthetic inputs</div>
            <h3>Baseline, then a change.</h3>
            <p>Imagine a warmed-up key that suddenly requests a new model only one second after its prior request.</p>
            {!showFindings ? (
                <button className="demo-run" onClick={() => setShowFindings(true)} data-testid="button-run-demo">
                <TerminalSquare size={15} /> Show illustrative example
              </button>
            ) : (
              <button className="demo-run" onClick={() => setShowFindings(false)} data-testid="button-reset-demo">
                <RotateCcw size={15} /> Reset scenario
              </button>
            )}
            <small>No network calls. No credentials, prompts, or actual keys used.</small>
          </div>
          <div className="demo-output" aria-live="polite">
              <div className="output-head"><span>Illustrative finding stream</span><span className="output-state">{showFindings ? 'illustration only' : 'Waiting for click'}</span></div>
            {showFindings ? (
              <div className="finding-list">
                <div className="finding"><span className="finding-icon"><Activity size={12} /></span><div><strong>rapid_sequence</strong><span>Interval is unusually short vs. synthetic baseline</span></div><span className="finding-score">+30</span></div>
                <div className="finding" style={{ animationDelay: '.08s' }}><span className="finding-icon"><TriangleAlert size={12} /></span><div><strong>new_model</strong><span>Model label not observed in this synthetic window</span></div><span className="finding-score">+25</span></div>
                <div className="finding" style={{ animationDelay: '.16s', borderLeftColor: '#69927c' }}><span className="finding-icon" style={{ background: '#dce8dd', color: '#3d6957' }}><Check size={12} /></span><div><strong>request_unchanged</strong><span>Illustrative pass-through status; no action taken</span></div><span className="finding-score" style={{ color: '#55826b' }}>observe</span></div>
              </div>
            ) : (
              <div className="empty-output"><b><Eye size={16} /></b>Nothing has run. Start the local scenario to reveal example signals.<br />No live data is being collected.</div>
            )}
          </div>
        </div>
        <div className="demo-disclaimer"><CircleHelp size={14} /> Entirely illustrative UI state. This page does not load FraudGuard, call an API, or calculate production scores.</div>
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
            <article className="privacy-item"><h3><Eye size={14} /> No prompt capture</h3><p>Findings omit prompts, responses, payloads, request IDs, and raw error text.</p></article>
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
            <p>To evaluate the prototype, copy the module into a matching Bifrost checkout, add it to that app’s module/workspace, construct it, and register it with <code>core.Init</code>.</p>
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

      <section className="section limits-section" id="scope">
        <div className="limits-layout">
          <div className="section-heading">
            <div className="section-kicker">Scope, plainly stated</div>
            <h2>Useful to inspect.<br /><em>Not magic.</em></h2>
            <p>This is an experiment with deliberately narrow claims. Use findings as review context, not as an automated verdict.</p>
          </div>
          <div className="limits-grid">
            <div className="limit"><TriangleAlert size={14} /> No dashboard, database, external AI service, or cross-process state.</div>
            <div className="limit"><TriangleAlert size={14} /> No automatic blocking, request edits, or authorization decision.</div>
            <div className="limit"><TriangleAlert size={14} /> No automatic payment, location, or key-status discovery.</div>
            <div className="limit"><TriangleAlert size={14} /> No prompt semantics, caller identity inference, or intent detection.</div>
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
          <a href="#privacy" data-testid="link-footer-boundaries">Data boundaries</a>
          <a href="#scope" data-testid="link-footer-limitations">Limits</a>
        </div>
        <span>OBSERVE ONLY · NO CREDENTIAL FORM</span>
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
