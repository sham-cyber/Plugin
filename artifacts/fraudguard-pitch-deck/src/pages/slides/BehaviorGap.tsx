import problemStatement from '@assets/fraudguard-hackathon-problem-statements.jpg';
import pluginDirectory from '@assets/fraudguard-bifrost-plugin-directory.png';

export default function BehaviorGap() {
  return (
    <div className="w-screen h-screen overflow-hidden relative bg-bg text-text font-body">
      <div className="absolute left-[6vw] top-[7vh] h-[1.1vh] w-[8vw] bg-accent" />
      <h1 className="absolute left-[6vw] top-[11vh] w-[72vw] font-display text-[3.7vw] font-bold leading-[1.08] tracking-[-0.035em] text-text">Request counts can miss behavior changes</h1>
      <div className="absolute left-[6vw] top-[28vh] w-[59vw] space-y-[3.2vh]">
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.2] text-text">Fixed request and token limits measure volume, not change from a key’s normal use</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.2] text-text">A key may stay below limits while probing models/providers or changing token use</p>
        </div>
        <div className="grid grid-cols-[1.1vw_1fr] gap-[1.3vw]">
          <span className="mt-[1.1vh] h-[0.78vw] w-[0.78vw] rounded-full bg-primary" />
          <p className="text-[2vw] leading-[1.2] text-text">In the inspected Bifrost commit, no dedicated per-key behavior detector was found</p>
        </div>
      </div>
      <div className="absolute right-[5vw] top-[24vh] flex w-[27vw] flex-col gap-[2vh]">
        <div className="flex h-[28vh] items-center justify-center overflow-hidden rounded-[1.1vw] border border-[var(--slide-line)] bg-[#15172b] p-[0.8vw] shadow-[0_1vw_2vw_rgba(24,49,57,0.10)]">
          <img src={problemStatement} crossOrigin="anonymous" alt="Attached Bifrost hackathon problem statement" className="h-full w-full rounded-[0.7vw] object-contain" />
        </div>
        <div className="flex h-[25vh] items-center justify-center overflow-hidden rounded-[1.1vw] border border-[var(--slide-line)] bg-white p-[0.8vw] shadow-[0_1vw_2vw_rgba(24,49,57,0.10)]">
          <img src={pluginDirectory} crossOrigin="anonymous" alt="Attached screenshot of the Bifrost plugin directory" className="h-full w-full rounded-[0.7vw] object-contain" />
        </div>
      </div>
      <p className="absolute bottom-[5.3vh] left-[6vw] w-[61vw] text-[1.5vw] leading-[1.25] text-muted">Source: Bifrost commit 54ad3abec4fbbdaedf59414415094760fb9aa7e9</p>
    </div>
  );
}
