import patentApplication from '@assets/IMG-20260819-WA0000_1793696679551.jpg';
import patentAbstract from '@assets/IMG-20260729-WA0019_1793696679703.jpg';

export default function PrototypeStatus() {
  return (
    <div className="w-screen h-screen overflow-hidden relative bg-bg text-text font-body">
      <div className="absolute left-[6vw] top-[7vh] h-[1.1vh] w-[8vw] bg-primary" />
      <h1 className="absolute left-[6vw] top-[11vh] w-[88vw] font-display text-[3.6vw] font-bold leading-[1.04] tracking-[-0.035em] text-text">A tested plugin prototype—not a built-in feature</h1>
      <div className="absolute left-[6vw] top-[25vh] w-[61vw] space-y-[2vh]">
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.16] text-text">Implements Bifrost's LLMPlugin; supplied through core.Init</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.16] text-text">Not in the built-in HTTP loader; not installed, accepted, or merged upstream</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.16] text-text">No live trusted feeds, production, replica, or latency validation; findings are heuristic</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.16] text-text">Hardening TODO (Issue #1): wire trusted payment/key feeds; deduplicate retries by idempotency ID</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.16] text-text">Patent reference IN202521074434 A1; cited only, not a claim of grant or coverage</p>
        </div>
      </div>
      <div className="absolute right-[5vw] top-[24vh] flex w-[27vw] flex-col gap-[2vh]">
        <div className="flex h-[28vh] items-center justify-center overflow-hidden rounded-[1.1vw] border border-[var(--slide-line)] bg-white p-[0.9vw] shadow-[0_1vw_2vw_rgba(24,49,57,0.10)]">
          <img src={patentApplication} crossOrigin="anonymous" alt="User-provided first page of the patent application" className="h-full w-full rounded-[0.7vw] object-contain" />
        </div>
        <div className="flex h-[28vh] items-center justify-center overflow-hidden rounded-[1.1vw] border border-[var(--slide-line)] bg-white p-[0.9vw] shadow-[0_1vw_2vw_rgba(24,49,57,0.10)]">
          <img src={patentAbstract} crossOrigin="anonymous" alt="User-provided patent application abstract screenshot" className="h-full w-full rounded-[0.7vw] object-contain" />
        </div>
      </div>
    </div>
  );
}
