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

function Home() {
  const [showFindings, setShowFindings] = useState(false);

  return (
    <main className="page-shell">
      <header className="topbar">
        <a href="#top" className="brand" aria-label="FraudGuard home" data-testid="link-home">
          <span className="brand-mark"><ShieldCheck size={17} strokeWidth={1.8} /></span>
          <span>FraudGuard <small>for Bifrost</small></span>
        </a>
        <nav className="nav-links" aria-label="Main navigation">
          <a href="#signals" data-testid="link-signals">Signals</a>
          <a href="#privacy" data-testid="link-privacy">Data boundaries</a>
          <a href="#integration" data-testid="link-integration">Integration</a>
          <a className="nav-cta" href="#demo" data-testid="link-demo">Try the local demo <ArrowDownRight size={14} /></a>
        </nav>
      </header>

      <section className="hero" id="top">
        <div>
          <div className="eyebrow">Experimental companion plugin · Observe-only</div>
          <h1>See the shift<br />before it <em>spreads.</em></h1>
          <p className="hero-copy">
            FraudGuard gives Bifrost operators a second view of authenticated virtual-key behavior—surfacing unusual changes for a human to investigate, while every request continues unchanged.
          </p>
          <div className="hero-actions">
            <a className="button-primary" href="#demo" data-testid="link-hero-demo">Explore a synthetic finding <ArrowRight size={15} /></a>
            <a className="button-secondary" href="#how-it-works" data-testid="link-how-it-works">How it works <ArrowDownRight size={15} /></a>
          </div>
          <p className="boundary-note">No request blocking. No prompt inspection. Not built into upstream Bifrost; this is a separate prototype module.</p>
        </div>

        <div className="terminal-wrap" aria-label="Illustrative synthetic behavior score">
          <div className="terminal">
            <div className="terminal-top">
              <span className="terminal-lights"><i /><i /><i /></span>
              <span>FRAUDGUARD / LOCAL EXAMPLE</span>
              <span>OBSERVE</span>
            </div>
            <div className="terminal-main">
              <div>
                <div className="score-label">Illustrative score</div>
                <div className="score-value">55<span> / 100</span></div>
                <div className="score-caption">Below 60 threshold · observe only</div>
              </div>
              <div className="sparkline" aria-hidden="true">{Array.from({ length: 10 }, (_, i) => <i key={i} />)}</div>
            </div>
            <div className="terminal-events">
              <div className="event-row"><span>rapid_sequence</span><strong>+30</strong><span>01s gap</span></div>
              <div className="event-row"><span>new_model</span><strong>+25</strong><span>unseen here</span></div>
            </div>
            <div className="synthetic-stamp"><span><i className="stamp-dot" />SYNTHETIC EVENT DATA</span><span>NO IDENTIFIERS</span></div>
          </div>
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
            <div className="section-kicker">A safe, local walkthrough</div>
            <h2>Make the behavior<br /><em>visible.</em></h2>
            <p>Run a tiny synthetic scenario in this page. It demonstrates the shape of a finding—not a live detector, Bifrost connection, or measured result.</p>
          </div>
        </div>
        <div className="demo-panel">
          <div className="demo-controls">
            <div className="micro">Local-only · Synthetic inputs</div>
            <h3>Baseline, then a change.</h3>
            <p>Imagine a warmed-up key that suddenly requests a new model only one second after its prior request.</p>
            {!showFindings ? (
              <button className="demo-run" onClick={() => setShowFindings(true)} data-testid="button-run-demo">
                <TerminalSquare size={15} /> Run synthetic scenario
              </button>
            ) : (
              <button className="demo-run" onClick={() => setShowFindings(false)} data-testid="button-reset-demo">
                <RotateCcw size={15} /> Reset scenario
              </button>
            )}
            <small>No network calls. No credentials, prompts, or actual keys used.</small>
          </div>
          <div className="demo-output" aria-live="polite">
              <div className="output-head"><span>Example finding stream</span><span className="output-state">{showFindings ? 'score 55 · alert=false' : 'Waiting for local run'}</span></div>
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
          <p>Walk through the local example, then inspect the companion module and decide whether its limits fit your environment.</p>
          <a className="button-primary" href="#demo" data-testid="link-final-demo">Run synthetic example <ArrowRight size={14} /></a>
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
