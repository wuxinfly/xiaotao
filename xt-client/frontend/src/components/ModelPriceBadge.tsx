import type {ModelPricing} from '../lib/api';
import './model-price-badge.css';

export function ModelPriceBadge({priceFactor, isFree}: ModelPricing) {
 const known = typeof priceFactor === 'number' && Number.isFinite(priceFactor) && priceFactor >= 0;
 if (isFree !== true && !known) return null;
 const free = isFree === true || (known && priceFactor === 0);
 return <small className={'model-price-badge' + (free ? ' is-free' : '')}
   title={free ? '上游标记免费或消耗倍率为零，以实际结算为准' : '上游消耗倍率，仅用于同渠道比较；以实际结算为准'}>
   {free ? '免费' : `${priceFactor}×`}
 </small>;
}
