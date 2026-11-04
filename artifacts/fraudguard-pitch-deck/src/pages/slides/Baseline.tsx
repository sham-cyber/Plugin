export default function Baseline() {
  return (
    <div className="w-screen h-screen overflow-hidden relative bg-bg text-text font-body">
      <div className="absolute left-[6vw] top-[7vh] h-[1.1vh] w-[8vw] bg-primary" />
      <h1 className="absolute left-[6vw] top-[11vh] w-[75vw] font-display text-[3.9vw] font-bold leading-[1.08] tracking-[-0.035em] text-text">Build a baseline, score deviations</h1>
      <div className="absolute left-[6vw] top-[28vh] w-[52vw] space-y-[3.2vh]">
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.2] text-text">Per-key baseline: model/provider mix, input/output tokens, request spacing, and error rate</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.2] text-text">Warmup and rolling window produce a deterministic 0–100 score with named reasons</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-accent" />
          <p className="text-[2vw] leading-[1.2] text-text">Observe-only: findings are reported; requests are never blocked or changed</p>
        </div>
      </div>
      <div className="absolute right-[6vw] top-[26vh] flex h-[62vh] w-[34vw] items-center justify-center overflow-hidden rounded-[1.8vw] bg-accent">
        <svg viewBox="0 0 640 640" className="absolute inset-0 h-full w-full" aria-hidden="true">
          <circle cx="320" cy="320" r="237" fill="none" stroke="#F1F5F2" strokeOpacity=".23" strokeWidth="3" />
          <circle cx="320" cy="320" r="184" fill="none" stroke="#F1F5F2" strokeOpacity=".35" strokeWidth="3" strokeDasharray="3 16" />
          <path d="M110 410C195 410 190 242 280 242H390C445 242 450 333 530 333" fill="none" stroke="#D6EEE2" strokeWidth="10" strokeLinecap="round" />
          <circle cx="110" cy="410" r="18" fill="#D6EEE2" />
          <circle cx="530" cy="333" r="18" fill="#D6EEE2" />
          <path d="M185 160V480M235 128V512M405 145V495M455 175V465" stroke="#F1F5F2" strokeOpacity=".22" strokeWidth="2" />
        </svg>
        <div className="relative z-10 -mt-[1vh] text-center">
          <p className="font-display text-[6.7vw] font-extrabold leading-none tracking-[-0.06em] text-bg">0–100</p>
        </div>
      </div>
    </div>
  );
}
