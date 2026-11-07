import {useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode, type RefObject} from 'react';
import {createPortal} from 'react-dom';
import './popover.css';

type Props = {
  open: boolean;
  onClose: () => void;
  anchorRef: RefObject<HTMLElement | null>;
  point?: {x: number; y: number};
  side?: 'top' | 'bottom';
  align?: 'start' | 'end';
  initialFocusRef?: RefObject<HTMLElement | null>;
  className?: string;
  id?: string;
  role?: 'menu' | 'group' | 'dialog';
  'aria-label': string;
  children: ReactNode;
};

/** Anchored, portaled surface. Business content remains with the caller. */
export function Popover({open, onClose, anchorRef, point, side = 'bottom', align = 'start', initialFocusRef, className = '', id, role = 'menu', children, ...aria}: Props) {
  const panel = useRef<HTMLDivElement>(null);
  const [mounted, setMounted] = useState(open);
  const [shown, setShown] = useState(false);
  const [position, setPosition] = useState<CSSProperties>({visibility: 'hidden'});
  const content = useRef(children);
  const close = useRef(onClose);
  const pointRef = useRef(point);
  close.current = onClose;
  pointRef.current = point;
  if (open) content.current = children;

  useEffect(() => {
    if (open) {setMounted(true); return;}
    setShown(false);
    const timer = window.setTimeout(() => setMounted(false), 160);
    return () => window.clearTimeout(timer);
  }, [open]);

  useLayoutEffect(() => {
    if (!mounted || !open) return;
    const element = panel.current;
    if (!element) return;
    const place = () => {
      const point = pointRef.current;
      const anchor = anchorRef.current?.getBoundingClientRect();
      if (!anchor && !point) return;
      const x = point?.x ?? (align === 'end' ? anchor!.right : anchor!.left);
      const top = point?.y ?? anchor!.top;
      const bottom = point?.y ?? anchor!.bottom;
      const height = element.offsetHeight;
      const above = top - 14;
      const below = window.innerHeight - bottom - 14;
      const actualSide = side === 'top' ? (above >= height || above >= below ? 'top' : 'bottom') : (below >= height || below >= above ? 'bottom' : 'top');
      const left = Math.max(8, Math.min(x - (align === 'end' && !point ? element.offsetWidth : 0), window.innerWidth - element.offsetWidth - 8));
      const available = Math.max(40, actualSide === 'top' ? above : below);
      const y = actualSide === 'top' ? top - Math.min(height, available) - 6 : bottom + 6;
      setPosition({left, top: Math.max(8, y), maxHeight: available, '--popover-shift': actualSide === 'top' ? '8px' : '-8px', transformOrigin: `${Math.max(0, Math.min(element.offsetWidth, (anchor ? anchor.left + anchor.width / 2 : x) - left))}px ${actualSide === 'top' ? '100%' : '0%'}`} as CSSProperties);
    };
    place();
    const observer = new ResizeObserver(place);
    observer.observe(element);
    window.addEventListener('resize', place);
    const scroll = (event: Event) => {if (!element.contains(event.target as Node)) place();};
    document.addEventListener('scroll', scroll, true);
    let revealFrame = 0;
    const frame = requestAnimationFrame(() => {revealFrame = requestAnimationFrame(() => setShown(true)); (initialFocusRef?.current ?? element.querySelector<HTMLElement>('[aria-checked="true"], [aria-pressed="true"], input, button:not(:disabled)'))?.focus({preventScroll: true});});
    return () => {cancelAnimationFrame(frame); cancelAnimationFrame(revealFrame); observer.disconnect(); window.removeEventListener('resize', place); document.removeEventListener('scroll', scroll, true);};
  }, [mounted, open, anchorRef, point?.x, point?.y, side, align, initialFocusRef]);

  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!panel.current?.contains(target) && !anchorRef.current?.contains(target)) close.current();
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {event.preventDefault(); event.stopPropagation(); close.current(); anchorRef.current?.focus({preventScroll: true});}
      if (event.key === 'Tab') close.current();
    };
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', key);
    return () => {document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', key);};
  }, [open, anchorRef]);

  if (!mounted) return null;
  return createPortal(<div ref={panel} id={id} role={role} {...aria} className={`app-popover ${className}${shown && open ? ' is-visible' : ''}`} style={position} inert={!open} aria-hidden={!open} onKeyDown={event => {
    if ((event.target as HTMLElement).matches('input, textarea') || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'));
    if (!buttons.length) return;
    event.preventDefault();
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length;
    buttons[next]?.focus();
  }}>{content.current}</div>, document.body);
}
