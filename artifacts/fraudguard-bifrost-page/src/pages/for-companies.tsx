import { ArrowDownRight, ArrowRight, Check, CircleHelp, Eye, ShieldCheck, TriangleAlert } from 'lucide-react';
import { Link } from 'wouter';

export default function ForCompanies() {
  return (
    <main className="page-shell company-page" data-testid="page-for-companies">
      <header className="topbar">
        <Link href="/" className="brand" aria-label="FraudGuard overview" data-testid="link-companies-brand">
          <span className="brand-mark"><ShieldCheck size={17} strokeWidth={1.8} /></span>
          <span>FraudGuard <small>for Bifrost</small></span>
        </Link>
        <nav className="nav-links company-nav" aria-label="Main navigation">
          <Link href="/" data-testid="link-companies-overview">Overview</Link>
          <Link href="/results" data-testid="link-companies-results">Demo results</Link>
          <Link href="/for-companies" aria-current="page" data-testid="link-companies-current">For companies</Link>
        </nav>
      </header>

      <section className="company-hero">
        <div className="company-hero-copy">
          <div className="eyebrow">A broader use case · proposed direction</div>
          <h1>Notice what<br />changes across<br /><em>your AI usage.</em></h1>
          <p>Organizations increasingly route AI requests through a gateway across applications and teams. The intended direction for FraudGuard is to help operators notice when that usage shifts—without reading prompts or changing requests.</p>
          <div className="company-hero-links">
            <Link href="/" className="button-primary" data-testid="link-company-overview">Review the Bifrost prototype <ArrowRight size={14} /></Link>
            <Link href="/results" className="company-text-link" data-testid="link-company-demo">See the synthetic demo <ArrowDownRight size={14} /></Link>
          </div>
        </div>
        <aside className="company-status-card" data-testid="status-current-scope">
          <div className="company-status-top"><span className="company-status-dot" /> CURRENT WORKING PROJECT</div>
          <strong>One experimental adapter.</strong>
          <p>A Go, observe-only plugin prototype specifically for Bifrost, supplied through <code>core.Init</code>.</p>
          <div className="company-status-rule" />
          <div className="company-status-bottom"><span>NOT A GENERAL GATEWAY PRODUCT</span><span>NOT PILOTED</span></div>
        </aside>
      </section>

      <section className="company-intent" data-testid="content-intended-use">
        <div>
          <div className="section-kicker">The intended company use case</div>
          <h2>One gateway view.<br /><em>Many ways to use AI.</em></h2>
        </div>
        <div className="company-intent-copy">
          <p>In a future deployment, an organization could use an AI gateway to route requests from its own applications and teams. FraudGuard’s broader goal is to help that organization identify unusual changes in its own usage patterns, relative to retained history.</p>
          <p className="company-note"><CircleHelp size={15} /> This is a direction, not functionality already delivered. The current prototype does not provide an organization-wide view, team dashboard, or live traffic feed.</p>
        </div>
      </section>

      <section className="company-signals section" data-testid="content-observed-changes">
        <div className="section-heading">
          <div className="section-kicker">What a behavior change can look like</div>
          <h2>Look for a shift.<br /><em>Not a verdict.</em></h2>
          <p>The Bifrost prototype compares request behavior with a virtual key’s retained history. Signals are review context; they do not establish intent.</p>
        </div>
        <div className="company-signal-list">
          <article><span>01</span><div><h3>New model or provider labels</h3><p>A label not seen in that virtual key’s retained history.</p></div></article>
          <article><span>02</span><div><h3>Token-volume changes</h3><p>Unusual input or output token volume when normalized usage is available.</p></div></article>
          <article><span>03</span><div><h3>Unusually fast request spacing</h3><p>Request timing that departs sharply from the key’s own recent interval pattern.</p></div></article>
          <article><span>04</span><div><h3>Repeated errors</h3><p>A repeated-failure pattern when those outcomes reach the plugin’s post-hook.</p></div></article>
        </div>
      </section>

      <section className="company-boundary" data-testid="content-privacy-boundary">
        <div className="company-boundary-heading">
          <div className="section-kicker">Operating boundary</div>
          <h2>Observe the change.<br /><em>Leave the request alone.</em></h2>
        </div>
        <div className="company-boundary-grid">
          <article><Eye size={17} /><h3>No prompt inspection</h3><p>The prototype stores no prompts and no raw credentials.</p></article>
          <article><ShieldCheck size={17} /><h3>Pass-through behavior</h3><p>Requests always pass through unchanged. The prototype does not block or rewrite them.</p></article>
          <article><TriangleAlert size={17} /><h3>Local, bounded history</h3><p>State is bounded and process-local. Multiple replicas do not share baselines; a process restart clears its in-memory history.</p></article>
          <article><CircleHelp size={17} /><h3>No live company view</h3><p>There is no live company dashboard or live customer traffic in this project.</p></article>
        </div>
      </section>

      <section className="company-plan section" data-testid="content-proposed-plan">
        <div className="company-plan-heading">
          <div className="section-kicker">Proposed work plan · not completed</div>
          <h2>Earn the next step<br /><em>in a test environment.</em></h2>
          <p>This plan describes work to consider before any limited observe-only rollout. It is not a record of completed company work.</p>
        </div>
        <ol className="company-plan-list">
          <li><span>01</span><div><h3>Agree on the target gateway and pilot scope</h3><p>Choose one gateway and define what a limited evaluation would include.</p></div></li>
          <li><span>02</span><div><h3>Arrange approved test access and key custody</h3><p>Use the organization’s approved process. Never enter secrets on this page.</p></div></li>
          <li><span>03</span><div><h3>Connect a limited test environment</h3><p>For another gateway, first build a compatible adapter and validate its integration.</p></div></li>
          <li><span>04</span><div><h3>Observe and tune a representative baseline</h3><p>Use representative activity and tune the retained-history window and alert behavior.</p></div></li>
          <li><span>05</span><div><h3>Test ordinary traffic and controlled shifts</h3><p>Include behavior changes that remain below fixed request-count limits.</p></div></li>
          <li><span>06</span><div><h3>Review operational and privacy questions</h3><p>Assess false positives, alert reasons, privacy and retention, latency, concurrency, and multi-replica behavior.</p></div></li>
          <li><span>07</span><div><h3>Decide whether observe-only rollout is ready</h3><p>Make a deliberate readiness decision before considering any limited rollout.</p></div></li>
        </ol>
        <div className="company-plan-foot"><Check size={15} /> Automated blocking is outside this prototype and is not part of this proposed rollout.</div>
      </section>

      <section className="company-gateway-note" data-testid="content-gateway-compatibility">
        <div className="company-gateway-mark"><ArrowRight size={17} /></div>
        <div><span className="section-kicker">Compatibility, stated plainly</span><h2>Bifrost today. Adapters by validation.</h2><p>The working project is specifically a Bifrost Go plugin prototype, supplied through <code>core.Init</code>. It is not built into upstream Bifrost and has not been piloted with a company. Other gateways are not supported by implication: each additional gateway needs a compatible adapter and its own validation.</p></div>
      </section>

      <section className="company-optional section" data-testid="content-optional-host-inputs">
        <div className="section-heading">
          <div className="section-kicker">Separate, optional inputs</div>
          <h2>Not required for<br /><em>the core use case.</em></h2>
        </div>
        <p>Payment checks and credential-status checks are separate optional trusted host inputs. They are not automatic capabilities, and neither is required to explore changes in an organization’s AI usage through a gateway.</p>
      </section>

      <footer className="company-footer">
        <span>FRAUDGUARD FOR BIFROST · CURRENTLY EXPERIMENTAL</span>
        <div><Link href="/" data-testid="link-company-footer-overview">Bifrost overview</Link><Link href="/results" data-testid="link-company-footer-results">Synthetic demo results</Link></div>
        <span>PROPOSED DIRECTION ≠ DELIVERED FUNCTIONALITY</span>
      </footer>
    </main>
  );
}
