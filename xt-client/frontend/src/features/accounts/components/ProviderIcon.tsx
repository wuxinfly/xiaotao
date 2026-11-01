import React from 'react';
import workbuddyIcon from '../../../assets/workbuddy-32.png';

interface ProviderIconProps {
  provider: string;
  size?: number;
  className?: string;
}

export const ProviderIcon: React.FC<ProviderIconProps> = ({ provider, size = 32, className = '' }) => {
  const p = (provider || '').toLowerCase();
  const traeClipId = React.useId().replace(/:/g, '');
  if (p === 'trae') {
    return (
      <svg
        width={(size * 4) / 3}
        height={size}
        viewBox="0 0 28 21"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className={`provider-icon provider-icon-trae ${className}`}
        aria-hidden="true"
      >
        <g clipPath={`url(#trae_logo_${traeClipId})`}>
          <path
            fill="currentColor"
            d="M28.002 20.846H4v-3.998H0V.846h28.002zM4 16.848h20.002V4.845H4zm10.002-6.062-2.829 2.828-2.828-2.828 2.828-2.829zm8-.002-2.828 2.828-2.829-2.828 2.829-2.829z"
          />
        </g>
        <defs>
          <clipPath id={`trae_logo_${traeClipId}`}>
            <path fill="currentColor" d="M0 .846h28.002v20H0z" />
          </clipPath>
        </defs>
      </svg>
    );
  }

  if (p === 'gemini') {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 32 32"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className={`provider-icon provider-icon-gemini ${className}`}
        aria-hidden="true"
      >
        <rect width="32" height="32" rx="7" fill="#0E2338" stroke="#1D4E6D" strokeWidth="1" />
        <path
          d="M16 6C16 11.5 20.5 16 26 16C20.5 16 16 20.5 16 26C16 20.5 11.5 16 6 16C11.5 16 16 11.5 16 6Z"
          fill="url(#gemini_grad)"
        />
        <defs>
          <linearGradient id="gemini_grad" x1="6" y1="6" x2="26" y2="26" gradientUnits="userSpaceOnUse">
            <stop stopColor="#4E82EE" />
            <stop offset="0.5" stopColor="#E25569" />
            <stop offset="1" stopColor="#F9AB00" />
          </linearGradient>
        </defs>
      </svg>
    );
  }

  if (p === 'workbuddy') {
    return (
      <img
        src={workbuddyIcon}
        alt=""
        width={size}
        height={size}
        className={`provider-icon provider-icon-workbuddy ${className}`}
        aria-hidden="true"
      />
    );
  }

  // Fallback generic proxy icon
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={`provider-icon provider-icon-generic ${className}`}
      aria-hidden="true"
    >
      <rect width="32" height="32" rx="7" fill="#143447" stroke="#255570" strokeWidth="1" />
      <circle cx="16" cy="16" r="6" stroke="#59DFDD" strokeWidth="2" strokeDasharray="3 3" />
      <circle cx="16" cy="16" r="2.5" fill="#59DFDD" />
    </svg>
  );
};
