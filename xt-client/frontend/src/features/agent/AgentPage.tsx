import './agent.css';

export function AgentPage() {
 return <section className="agent-page" aria-labelledby="agent-title">
  <div className="agent-content">
   <img className="agent-mascot" src="/agent/mascot-ripple.png" alt="小涛水滴精灵，跃于粼粼水波之上" draggable={false} />
   <div className="agent-copy">
    <span className="agent-status"><span aria-hidden="true" />正在用心打磨</span>
    <h1 id="agent-title">下一程，<span>一起出发。</span></h1>
    <p>从一个想法，到一次实现。<br />小涛 Agent，你的 AI 协作伙伴，即将就位。</p>
    <div className="agent-coming"><span aria-hidden="true" /><span>敬请期待 <b aria-hidden="true">·</b> COMING SOON</span><span aria-hidden="true" /></div>
   </div>
  </div>
 </section>;
}
