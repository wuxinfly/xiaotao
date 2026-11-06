import {useEffect, useRef, useState} from 'react';
import {BookOpen, FolderOpen, Plus, RefreshCw, Trash2} from 'lucide-react';
import {api, type SkillsCatalog, type SkillsSettings as Settings} from '../../lib/api';
import './skills-settings.css';

const empty: Settings = {directories: [], disabledSkills: []};
const message = (reason: unknown) => reason instanceof Error ? reason.message : String(reason);

export function SkillsSettings({active, onBusyChange}: {active: boolean; onBusyChange: (busy: boolean) => void}) {
  const [settings, setSettings] = useState<Settings>(empty);
  const [catalog, setCatalog] = useState<SkillsCatalog | null>(null);
  const [loaded, setLoaded] = useState(false), [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(false), [busy, setBusy] = useState<'scan' | 'save' | null>(null);
  const [picking, setPicking] = useState(false), [dirty, setDirty] = useState(false), [needsScan, setNeedsScan] = useState(false);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const controller = useRef<AbortController | null>(null);
  const pickerPending = useRef(false);
  const running = useRef(false);
  const blocked = busy !== null || picking;
  useEffect(() => {onBusyChange(blocked);}, [blocked, onBusyChange]);
  useEffect(() => () => controller.current?.abort(), []);
  useEffect(() => {
    if (!active || loaded) return;
    const request = new AbortController();
    setLoading(true); setError('');
    void api.agentSkills(request.signal).then(result => {
      setSettings(result.settings); setCatalog(result); setLoaded(true); setDirty(false); setNeedsScan(false);
    }).catch(reason => {if (!request.signal.aborted) setError(message(reason));})
      .finally(() => {if (!request.signal.aborted) setLoading(false);});
    return () => request.abort();
  }, [active, loaded, attempt]);

  function addDirectory(path = '') {
    setSettings(current => ({...current, directories: [...current.directories, {path}]}));
    setDirty(true); setNeedsScan(true); setError(''); setNotice('');
  }
  useEffect(() => {
    if (!active) return;
    const previous = window.xtFolderPicked;
    const handler = (payload: {path?: string; error?: string}) => {
      if (!pickerPending.current) return;
      pickerPending.current = false; setPicking(false);
      if (payload.error && !/取消|cancel/i.test(payload.error)) setError(payload.error);
      if (payload.path) addDirectory(payload.path);
    };
    window.xtFolderPicked = handler;
    return () => {if (window.xtFolderPicked === handler) window.xtFolderPicked = previous;};
  }, [active]);

  function updateDirectory(index: number, patch: Partial<Settings['directories'][number]>) {
    setSettings(current => ({...current, directories: current.directories.map((dir, i) => i === index ? {...dir, ...patch} : dir)}));
    setDirty(true); setNeedsScan(true); setNotice(''); setError('');
  }
  async function pick() {
    setError(''); setPicking(true);
    if (window.xtPickFolderOnly) {
      pickerPending.current = true;
      try {window.xtPickFolderOnly();} catch (reason) {pickerPending.current = false; setPicking(false); setError(message(reason));}
      return;
    }
    try {
      if (!window.xtPickFolder) throw new Error('浏览器无法读取文件夹路径，请直接填写绝对路径。');
      const path = await window.xtPickFolder(); if (path) addDirectory(path);
    } catch (reason) {if (!/取消|cancel/i.test(message(reason))) setError(message(reason));}
    finally {setPicking(false);}
  }
  async function operate(mode: 'scan' | 'save', input = settings) {
    if (running.current) return;
    if (input.directories.some(dir => !dir.path.trim())) {setError('请填写技能目录的绝对路径，或移除空目录。'); return;}
    running.current = true; setBusy(mode); setError(''); setNotice('');
    const request = new AbortController(); controller.current = request;
    const timeout = window.setTimeout(() => request.abort(), 30000);
    try {
      const result = await (mode === 'save' ? api.saveAgentSkills(input, request.signal) : api.scanAgentSkills(input, request.signal));
      setSettings(result.settings); setCatalog(result); setNeedsScan(false);
      if (mode === 'save') {setDirty(false); setNotice('已保存，下一次发送消息生效。');}
      else setNotice('扫描完成，尚未保存的修改不会生效。');
    } catch (reason) {
      setError(request.signal.aborted ? '请求未完成，请重新读取已保存配置确认结果。' : message(reason));
    } finally {window.clearTimeout(timeout); controller.current = null; running.current = false; setBusy(null);}
  }
  function toggleSkill(id: string, enabled: boolean) {
    const next = {...settings, disabledSkills: enabled ? settings.disabledSkills.filter(item => item !== id) : [...settings.disabledSkills, id]};
    setSettings(next); setDirty(true); void operate('scan', next);
  }

  return <div className="skills-settings">
    <div className="skills-core"><BookOpen aria-hidden="true"/><div><strong>小涛 Core <span>始终启用</span></strong><p>小涛的人设和基础协作规则默认加载，下面的技能为它补充能力。</p></div></div>
    <p className="skills-intro">添加单个技能目录，或包含多个技能的目录。小涛先看名称和描述，需要时再读取完整说明。</p>
    <p className="skills-trust">仅添加你信任的技能。扫描不会运行脚本；使用时沿用当前访问模式，缺少依赖会说明。</p>
    {loading && <p role="status">正在读取 Skills 配置…</p>}
    {error && <p className="agent-modal-error" role="alert">{error}</p>}
    {!loaded && !loading && error && <button type="button" className="app-modal-button secondary" onClick={() => setAttempt(value => value + 1)}>重试读取技能配置</button>}
    {loaded && <>
      <div className="skills-toolbar"><button type="button" className="app-modal-button secondary" disabled={blocked || settings.directories.length >= 16} onClick={() => addDirectory()}><Plus aria-hidden="true"/>添加目录</button>
        {(window.xtPickFolder || window.xtPickFolderOnly) && <button type="button" className="app-modal-button secondary" disabled={blocked || settings.directories.length >= 16} onClick={() => void pick()}><FolderOpen aria-hidden="true"/>{picking ? '选择中…' : '选择目录'}</button>}
        <button type="button" className="app-modal-button quiet" disabled={blocked} onClick={() => void operate('scan')}><RefreshCw aria-hidden="true" className={busy === 'scan' ? 'skills-spinning' : ''}/>{busy === 'scan' ? '扫描中…' : '扫描目录'}</button>
      </div>
      {settings.directories.map((dir, index) => <section className="skills-directory" key={index} aria-label={`技能目录 ${index + 1}`}>
        <div className="skills-directory-head"><label><input type="checkbox" checked={!dir.disabled} disabled={blocked} onChange={event => updateDirectory(index, {disabled: !event.target.checked})}/>启用目录 {index + 1}</label><button type="button" className="app-modal-button quiet" aria-label={`移除技能目录 ${index + 1}`} disabled={blocked} onClick={() => {setSettings(current => ({...current, directories: current.directories.filter((_, i) => i !== index)})); setDirty(true); setNeedsScan(true); setNotice('');}}><Trash2 aria-hidden="true"/>移除</button></div>
        <label className="agent-field"><span>技能目录路径 {index + 1}</span><input value={dir.path} disabled={blocked} onChange={event => updateDirectory(index, {path: event.target.value})} placeholder="例如 C:\Users\你的用户名\.agents\skills" spellCheck={false}/></label>
      </section>)}
      {!settings.directories.length && <div className="skills-empty"><FolderOpen aria-hidden="true"/><strong>还没有添加本地技能</strong><span>目录中需要有 SKILL.md，或在它的一级子目录中。</span></div>}
      {needsScan && <p className="skills-hint" role="status">目录已更改，请扫描后确认技能，或直接保存并扫描。</p>}
      {!needsScan && catalog && <>
        <div className="skills-results-head"><strong>已发现 {catalog.skills.length} 个技能</strong><span>{catalog.skills.filter(skill => skill.enabled).length} 个可用</span></div>
        {catalog.issues.map((issue, index) => <div className="skills-issue" role="status" key={`${issue.path}-${index}`}><strong>{issue.message}</strong><code>{issue.path}</code></div>)}
        {!!catalog.issuesOmitted && <p className="skills-hint">还有 {catalog.issuesOmitted} 条诊断未展示，请缩小扫描目录。</p>}
        {catalog.skills.map(skill => <section className={'skills-card' + (!skill.enabled ? ' is-disabled' : '')} key={skill.id} aria-label={`技能 ${skill.name}`}>
          <div className="skills-card-head"><strong>{skill.name}</strong><label><input type="checkbox" checked={!settings.disabledSkills.includes(skill.id)} disabled={blocked || /内置小涛/.test(skill.error || '')} onChange={event => toggleSkill(skill.id, event.target.checked)}/>启用 {skill.name}</label></div>
          <p>{skill.description}</p><code>{skill.path}</code>{skill.error && <p className="skills-conflict" role="status">{skill.error}</p>}
        </section>)}
      </>}
      <div className="skills-save"><span role="status">{notice || (dirty ? '有未保存的修改' : '配置仅用于当前客户端环境。')}</span><button type="button" className="app-modal-button primary" disabled={blocked} onClick={() => void operate('save')}>{busy === 'save' ? '保存中…' : '保存 Skills 配置'}</button></div>
    </>}
  </div>;
}
