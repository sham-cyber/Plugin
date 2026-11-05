export default function Evidence() {
  return (
    <div className="w-screen h-screen overflow-hidden relative bg-bg text-text font-body">
      <div className="absolute left-[6vw] top-[7vh] h-[1.1vh] w-[8vw] bg-accent" />
      <h1 className="absolute left-[6vw] top-[11vh] w-[80vw] font-display text-[4vw] font-bold leading-[1.08] tracking-[-0.035em] text-text">Evidence with privacy built in</h1>
      <div className="absolute left-[6vw] top-[27vh] w-[61vw] space-y-[2.6vh]">
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.18] text-text">Findings omit prompts, payloads, raw keys, and payment IDs</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.18] text-text">Bounded memory; HMAC cross-key correlation off by default</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.18] text-text">Passed Go tests, race, vet (Go 1.27.3; 2026-11-04)</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.18] text-text">Demo: behavior 30/no alert; payment 95/alert; key status 100/alert</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.18] text-text">Dashboard: AshaRatna Tech is illustrative; subscription simulated; email/SMS previews not sent</p>
        </div>
      </div>
      <div className="absolute right-[6vw] top-[27vh] flex h-[58vh] w-[26vw] flex-col justify-center rounded-[1.6vw] bg-primary px-[2.2vw] text-bg shadow-[0_1.2vw_2.4vw_rgba(24,49,57,0.14)]">
        <p className="font-display text-[9vw] font-extrabold leading-[0.86] tracking-[-0.07em]">30</p>
        <div className="mt-[4vh] h-[0.7vh] w-[4vw] bg-[#a7d5bf]" />
        <p className="mt-[3vh] text-[2.1vw] font-semibold leading-[1.25]">
          <span className="block">score=30</span>
          <span className="block">alert=false</span>
          <span className="block">signals=[rapid_sequence]</span>
        </p>
      </div>
    </div>
  );
}
