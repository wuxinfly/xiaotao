import {useState} from 'react';
import {createRoot} from 'react-dom/client';
import {ModalFrame} from '../src/components/modal/ModalFrame';
import {Select} from '../src/components/select/Select';
import {applyTheme} from '../src/lib/theme';
import '../src/style.css';
import '../src/ocean-tech.css';
import '../src/shallow-sea.css';

applyTheme(new URLSearchParams(location.search).get('theme') === 'light' ? 'light' : 'dark');

const models = Array.from({length: 24}, (_, index) => ({
  value: `model-${index + 1}`,
  label: `模型 ${String(index + 1).padStart(2, '0')}`,
  hint: index % 4 === 0 ? '内置' : undefined,
}));

const basics = [{value: 'alpha', label: 'Alpha'}, {value: 'beta', label: 'Beta'}, {value: 'gamma', label: 'Gamma'}];

function Fixture() {
  const [basic, setBasic] = useState('beta');
  const [searched, setSearched] = useState('model-3');
  const [modalValue, setModalValue] = useState('alpha');
  const [modalOpen, setModalOpen] = useState(false);
  return <div className="fixture">
    <label><span>基础下拉</span>
      <Select aria-label="基础下拉" value={basic} onChange={setBasic} options={basics}/>
    </label>
    <output id="basic-value">{basic}</output>

    <label><span>搜索下拉</span>
      <Select aria-label="搜索下拉" searchable value={searched} onChange={setSearched} options={models}/>
    </label>
    <output id="search-value">{searched}</output>

    <label><span>禁用下拉</span>
      <Select aria-label="禁用下拉" disabled value="locked" onChange={() => undefined} options={[{value: 'locked', label: '已锁定'}]}/>
    </label>

    <label><span>空下拉</span>
      <Select aria-label="空下拉" value="" onChange={() => undefined} options={[]}/>
    </label>

    <button id="open-modal" type="button" onClick={() => setModalOpen(true)}>打开弹窗</button>

    <div className="fixture-bottom">
      <label><span>底部下拉</span>
        <Select aria-label="底部下拉" value="one" onChange={() => undefined} options={[{value: 'one', label: '选项一'}, {value: 'two', label: '选项二'}]}/>
      </label>
    </div>

    {modalOpen && <ModalFrame title="弹窗内下拉" onClose={() => setModalOpen(false)} footer={<button type="button" onClick={() => setModalOpen(false)}>关闭</button>}>
      <label><span>弹窗下拉</span>
        <Select aria-label="弹窗下拉" value={modalValue} onChange={setModalValue} options={basics}/>
      </label>
      <output id="modal-value">{modalValue}</output>
    </ModalFrame>}
  </div>;
}

createRoot(document.getElementById('root')!).render(<Fixture/>);
