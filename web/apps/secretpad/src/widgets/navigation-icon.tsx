import React from 'react';

/** Small local SVG vocabulary: consistent strokes without loading an icon font
 * or a new runtime dependency. Icons are decorative; links carry text labels. */
export function NavigationIcon({ name, className = 'h-[18px] w-[18px]' }: { name: string; className?: string }) {
  const paths: Record<string, React.ReactNode> = {
    projects: <><path d="M3 7h7l2-3h8v15H3z" /><path d="M3 7V4h6l3 3" /></>,
    data: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v7c0 4 16 4 16 0V5M4 12v7c0 4 16 4 16 0v-7" /></>,
    nodes: <><rect x="3" y="3" width="7" height="7" rx="2" /><rect x="14" y="14" width="7" height="7" rx="2" /><path d="M14 6h4v8M6 10v8h8" /></>,
    dag: <><rect x="3" y="8" width="6" height="8" rx="2" /><rect x="15" y="2" width="6" height="7" rx="2" /><rect x="15" y="15" width="6" height="7" rx="2" /><path d="m9 12 6-6M9 12l6 6" /></>,
    templates: <><rect x="3" y="3" width="7" height="7" rx="2" /><rect x="14" y="3" width="7" height="7" rx="2" /><rect x="3" y="14" width="7" height="7" rx="2" /><path d="M14 14h7v7h-7zM16 17h3" /></>,
    messages: <><path d="M4 4h16v13H9l-5 4z" /><path d="M8 9h8M8 13h5" /></>,
    clock: <><circle cx="12" cy="12" r="9" /><path d="M12 6v6l4 2" /></>,
    shield: <><path d="m12 2 8 3v7c0 5-8 10-8 10S4 17 4 12V5z" /><path d="m8 12 3 3 5-6" /></>,
    home: <><rect x="3" y="3" width="7" height="9" rx="2" /><rect x="14" y="3" width="7" height="5" rx="2" /><rect x="3" y="16" width="7" height="5" rx="2" /><rect x="14" y="12" width="7" height="9" rx="2" /></>,
    menu: <path d="M4 6h16M4 12h16M4 18h16" />,
    bell: <><path d="M5 17h14l-2-3V9a5 5 0 0 0-10 0v5zM10 21h4" /></>,
  };
  const key = name in paths ? name : /project/.test(name) ? 'projects' : /data|feature/.test(name) ? 'data' : /template/.test(name) ? 'templates' : /dag|graph/.test(name) ? 'dag' : /node|institution/.test(name) ? 'nodes' : /message/.test(name) ? 'messages' : /periodic|job|log/.test(name) ? 'clock' : /audit|privacy/.test(name) ? 'shield' : 'home';
  return <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[key]}</svg>;
}
