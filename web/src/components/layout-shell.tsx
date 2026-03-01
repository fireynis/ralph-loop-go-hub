'use client';

import { useWebSocket } from '@/hooks/useWebSocket';
import Link from 'next/link';

export function LayoutShell({ children }: { children: React.ReactNode }) {
  const wsUrl =
    typeof window !== 'undefined'
      ? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws`
      : '';

  useWebSocket(wsUrl);

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <nav className="border-b border-gray-800 px-6 py-4">
        <div className="flex items-center gap-6">
          <Link href="/" className="text-xl font-bold">
            Ralph Hub
          </Link>
          <Link href="/sessions" className="text-gray-400 hover:text-white">
            Sessions
          </Link>
          <Link href="/settings" className="text-gray-400 hover:text-white">
            Settings
          </Link>
        </div>
      </nav>
      <main className="p-6">{children}</main>
    </div>
  );
}
