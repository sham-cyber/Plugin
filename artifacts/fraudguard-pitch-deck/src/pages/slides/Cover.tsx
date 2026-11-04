export default function Cover() {
  return (
    <div className="w-screen h-screen overflow-hidden relative bg-bg text-text font-body">
      <div className="absolute inset-y-0 right-0 w-[43vw] bg-primary" />
      <div className="absolute right-[4.5vw] top-[10vh] h-[80vh] w-[35vw] overflow-hidden rounded-[1.6vw] border border-white/30 bg-primary">
        <svg viewBox="0 0 720 900" className="h-full w-full" aria-hidden="true">
          <path d="M0 150H720M0 300H720M0 450H720M0 600H720M0 750H720" stroke="#FFFFFF" strokeOpacity=".10" strokeWidth="2" />
          <path d="M120 0V900M270 0V900M420 0V900M570 0V900" stroke="#FFFFFF" strokeOpacity=".08" strokeWidth="2" />
          <path d="M55 450H205C255 450 252 305 302 305H360" fill="none" stroke="#DDECE5" strokeWidth="6" strokeLinecap="round" />
          <path d="M55 220H170C230 220 220 375 286 375H355" fill="none" stroke="#A7D5BF" strokeWidth="5" strokeLinecap="round" />
          <path d="M55 690H175C232 690 220 525 286 525H355" fill="none" stroke="#DDECE5" strokeWidth="5" strokeLinecap="round" />
          <circle cx="55" cy="450" r="14" fill="#A7D5BF" />
          <circle cx="55" cy="220" r="10" fill="#F1F5F2" />
          <circle cx="55" cy="690" r="10" fill="#F1F5F2" />
          <circle cx="410" cy="450" r="170" fill="none" stroke="#F1F5F2" strokeOpacity=".72" strokeWidth="4" />
          <circle cx="410" cy="450" r="126" fill="none" stroke="#A7D5BF" strokeOpacity=".95" strokeWidth="28" strokeDasharray="530 270" transform="rotate(-94 410 450)" />
          <circle cx="410" cy="450" r="83" fill="#214cb8" stroke="#F1F5F2" strokeOpacity=".85" strokeWidth="3" />
          <path d="M493 450H600C630 450 635 420 665 420" fill="none" stroke="#A7D5BF" strokeWidth="6" strokeLinecap="round" />
          <circle cx="665" cy="420" r="13" fill="#A7D5BF" />
          <rect x="540" y="275" width="94" height="22" rx="11" fill="#F1F5F2" fillOpacity=".20" />
          <rect x="540" y="318" width="126" height="14" rx="7" fill="#F1F5F2" fillOpacity=".48" />
          <rect x="540" y="575" width="108" height="18" rx="9" fill="#A7D5BF" fillOpacity=".80" />
          <rect x="540" y="615" width="76" height="14" rx="7" fill="#F1F5F2" fillOpacity=".35" />
        </svg>
      </div>
      <div className="relative z-10 flex h-full items-center">
        <div className="ml-[6.5vw] w-[51vw] pr-[2vw]">
          <h1 className="font-display text-[6.3vw] font-extrabold leading-[0.98] tracking-[-0.055em] text-text">FraudGuard</h1>
          <p className="mt-[5vh] max-w-[47vw] text-[2.7vw] font-semibold leading-[1.13] text-text">Behavior-based abuse signals for AI gateways</p>
          <p className="mt-[2.4vh] max-w-[44vw] text-[2.1vw] leading-[1.3] text-muted">An observe-only Go plugin prototype for Bifrost</p>
        </div>
      </div>
      <div className="absolute bottom-0 left-0 h-[1.2vh] w-[57vw] bg-accent" />
    </div>
  );
}
