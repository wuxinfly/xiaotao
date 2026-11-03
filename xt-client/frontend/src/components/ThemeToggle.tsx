import {useEffect, useState} from 'react';
import {Moon, Sun} from 'lucide-react';
import {readTheme, saveTheme} from '../lib/theme';

export function ThemeToggle() {
  const [theme, setTheme] = useState(readTheme);
  useEffect(() => {
    const sync = () => setTheme(readTheme());
    window.addEventListener('xiaotao:theme', sync);
    return () => window.removeEventListener('xiaotao:theme', sync);
  }, []);
  const label = theme === 'dark' ? '切换到浅海亮色主题' : '切换到深海暗色主题';
  return <button type="button" className="theme-toggle" aria-label={label} title={label}
    onClick={() => {const next = theme === 'dark' ? 'light' : 'dark'; saveTheme(next); setTheme(next);}}>
    {theme === 'dark' ? <Sun aria-hidden="true"/> : <Moon aria-hidden="true"/>}
    <span>{theme === 'dark' ? '浅海' : '深海'}</span>
  </button>;
}
