import type { SVGProps } from 'react'

export type IconName =
  | 'arrow-left'
  | 'bookmark'
  | 'check'
  | 'chevron-right'
  | 'close'
  | 'drag-handle'
  | 'menu'
  | 'pause'
  | 'play'
  | 'refresh'
  | 'rss'
  | 'star'
  | 'tag'
  | 'trash'

interface IconProps extends SVGProps<SVGSVGElement> {
  name: IconName
  size?: number
}

const paths: Record<IconName, React.ReactNode> = {
  'arrow-left': <path d="m15 18-6-6 6-6" />,
  bookmark: <path d="M6 4.75A1.75 1.75 0 0 1 7.75 3h8.5A1.75 1.75 0 0 1 18 4.75V21l-6-3.6L6 21V4.75Z" />,
  check: <path d="m5 12 4 4L19 6" />,
  'chevron-right': <path d="m9 18 6-6-6-6" />,
  close: <path d="M18 6 6 18M6 6l12 12" />,
  'drag-handle': <path d="M5 7h14M5 12h14M5 17h14" />,
  menu: <path d="M5 8h14M5 16h14" />,
  pause: <path d="M8 5v14M16 5v14" />,
  play: <path d="m8 5 11 7-11 7V5Z" />,
  refresh: <path d="M20 11a8 8 0 1 0-2.34 5.66M20 4v7h-7" />,
  rss: <><path d="M5 11a8 8 0 0 1 8 8M5 5a14 14 0 0 1 14 14" /><circle cx="6" cy="18" r="1" fill="currentColor" stroke="none" /></>,
  star: <path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3l-5.6 2.9 1.1-6.2L3 9.6l6.2-.9L12 3Z" />,
  tag: <path d="M20 13 13 20 4 11V4h7l9 9ZM8 8h.01" />,
  trash: <><path d="M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14M10 11v6M14 11v6" /></>,
}

export function Icon({ name, size = 20, ...props }: IconProps) {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height={size}
      viewBox="0 0 24 24"
      width={size}
      {...props}
    >
      <g stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8">
        {paths[name]}
      </g>
    </svg>
  )
}
