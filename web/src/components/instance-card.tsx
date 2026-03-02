'use client';

import type { InstanceState } from '@/lib/types';
import { useState, useEffect } from 'react';

interface InstanceCardProps {
  instance: InstanceState;
}

function formatRelativeTime(timestamp: string): string {
  const now = Date.now();
  const then = new Date(timestamp).getTime();
  const diffMs = now - then;

  if (diffMs < 0) return 'just now';

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return `${seconds}s ago`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function useRelativeTime(timestamp: string): string {
  const [text, setText] = useState(() => formatRelativeTime(timestamp));

  useEffect(() => {
    setText(formatRelativeTime(timestamp));

    const age = Date.now() - new Date(timestamp).getTime();
    // Tick every second for <60s, every 30s for <1h, every 60s otherwise
    const interval = age < 60_000 ? 1_000 : age < 3_600_000 ? 30_000 : 60_000;

    const id = setInterval(() => setText(formatRelativeTime(timestamp)), interval);
    return () => clearInterval(id);
  }, [timestamp]);

  return text;
}

function getPassRate(instance: InstanceState): number | null {
  const analytics = instance.context?.analytics;
  if (!analytics) return null;
  const total = analytics.passed_count + analytics.failed_count;
  if (total === 0) return null;
  return analytics.passed_count / total;
}

function getBorderColor(instance: InstanceState): string {
  if (instance.status === 'ended') return 'border-l-gray-500';

  if (instance.status === 'running') {
    const passRate = getPassRate(instance);
    if (passRate !== null && passRate < 0.7) return 'border-l-amber-500';
    return 'border-l-emerald-500';
  }

  return 'border-l-gray-500';
}

export function InstanceCard({ instance }: InstanceCardProps) {
  const passRate = getPassRate(instance);
  const borderColor = getBorderColor(instance);
  const context = instance.context;
  const lastEventAgo = useRelativeTime(instance.last_event);

  return (
    <a href={`/instances/${encodeURIComponent(instance.instance_id)}`}>
      <div
        className={`rounded-lg bg-gray-800 border-l-4 ${borderColor} p-4 hover:bg-gray-750 transition-colors cursor-pointer`}
      >
        <div className="flex items-start justify-between mb-2">
          <h3 className="text-lg font-bold text-gray-100 truncate">
            {instance.repo}
          </h3>
          <span className="text-xs text-gray-500 whitespace-nowrap ml-2">
            {lastEventAgo}
          </span>
        </div>

        {instance.epic && (
          <p className="text-sm text-gray-400 mb-3 truncate">{instance.epic}</p>
        )}

        <div className="grid grid-cols-2 gap-2 text-sm">
          {context?.current_phase && (
            <div>
              <span className="text-gray-500">Phase</span>
              <p className="text-gray-300 font-medium">{context.current_phase}</p>
            </div>
          )}

          {context && (
            <div>
              <span className="text-gray-500">Iteration</span>
              <p className="text-gray-300 font-medium">
                {context.current_iteration} / {context.max_iterations}
              </p>
            </div>
          )}

          {passRate !== null && (
            <div>
              <span className="text-gray-500">Pass Rate</span>
              <p className={`font-medium ${passRate >= 0.7 ? 'text-emerald-400' : 'text-amber-400'}`}>
                {Math.round(passRate * 100)}%
              </p>
            </div>
          )}

          {context?.analytics && (
            <div>
              <span className="text-gray-500">Tasks Closed</span>
              <p className="text-gray-300 font-medium">
                {context.analytics.tasks_closed}
              </p>
            </div>
          )}
        </div>
      </div>
    </a>
  );
}
