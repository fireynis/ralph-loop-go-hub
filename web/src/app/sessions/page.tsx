'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import type { Session } from '@/lib/types';

const PAGE_SIZE = 20;

function getApiBase(): string {
  if (typeof window === 'undefined') return '';
  return window.location.origin;
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleString();
}

function passRateColor(rate: number): string {
  if (rate >= 0.8) return 'text-green-400';
  if (rate >= 0.5) return 'text-yellow-400';
  return 'text-red-400';
}

export default function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [offset, setOffset] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);

  async function fetchSessions(currentOffset: number, append: boolean) {
    try {
      const res = await fetch(
        `${getApiBase()}/api/v1/sessions?offset=${currentOffset}&limit=${PAGE_SIZE}`
      );
      if (!res.ok) throw new Error(`API error: ${res.status}`);
      const data: Session[] = await res.json();
      setSessions((prev) => (append ? [...prev, ...data] : data));
      setHasMore(data.length === PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch sessions');
    }
  }

  useEffect(() => {
    setLoading(true);
    fetchSessions(0, false).finally(() => setLoading(false));
  }, []);

  async function handleLoadMore() {
    const nextOffset = offset + PAGE_SIZE;
    setLoadingMore(true);
    await fetchSessions(nextOffset, true);
    setOffset(nextOffset);
    setLoadingMore(false);
  }

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <div className="mx-auto max-w-6xl px-6 py-12">
        <h1 className="text-3xl font-bold tracking-tight text-white">
          Sessions
        </h1>
        <p className="mt-1 text-sm text-gray-400">
          All recorded ralph sessions.
        </p>

        <div className="mt-8">
          {loading && (
            <p className="text-sm text-gray-400">Loading...</p>
          )}

          {error && (
            <div className="rounded-lg border border-red-800 bg-red-900/30 px-4 py-3">
              <p className="text-sm text-red-300">{error}</p>
            </div>
          )}

          {!loading && !error && sessions.length === 0 && (
            <div className="rounded-lg border border-gray-700 bg-gray-800 px-6 py-8 text-center">
              <p className="text-sm text-gray-400">
                No sessions recorded yet.
              </p>
            </div>
          )}

          {!loading && !error && sessions.length > 0 && (
            <>
              <div className="overflow-x-auto rounded-lg border border-gray-700">
                <table className="w-full text-left text-sm">
                  <thead className="bg-gray-800 text-xs uppercase tracking-wide text-gray-400">
                    <tr>
                      <th className="px-4 py-3">Repo</th>
                      <th className="px-4 py-3">Epic</th>
                      <th className="px-4 py-3">Started</th>
                      <th className="px-4 py-3">Ended</th>
                      <th className="px-4 py-3 text-right">Iterations</th>
                      <th className="px-4 py-3 text-right">Tasks Closed</th>
                      <th className="px-4 py-3 text-right">Pass Rate</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-700 bg-gray-800">
                    {sessions.map((session) => (
                      <tr
                        key={session.session_id}
                        className="transition-colors hover:bg-gray-750 hover:bg-gray-700/50"
                      >
                        <td className="px-4 py-3">
                          <Link
                            href={`/sessions/${session.session_id}`}
                            className="font-medium text-blue-400 hover:text-blue-300 hover:underline"
                          >
                            {session.repo}
                          </Link>
                        </td>
                        <td className="px-4 py-3 text-gray-300">
                          {session.epic || '\u2014'}
                        </td>
                        <td className="px-4 py-3 text-gray-300">
                          {formatDate(session.started_at)}
                        </td>
                        <td className="px-4 py-3">
                          {session.ended_at ? (
                            <span className="text-gray-300">
                              {formatDate(session.ended_at)}
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1.5 text-green-400">
                              <span className="inline-block h-2 w-2 rounded-full bg-green-400" />
                              Running
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums text-gray-300">
                          {session.iterations}
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums text-gray-300">
                          {session.tasks_closed}
                        </td>
                        <td
                          className={`px-4 py-3 text-right tabular-nums font-medium ${passRateColor(session.pass_rate)}`}
                        >
                          {(session.pass_rate * 100).toFixed(0)}%
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>

              {hasMore && (
                <div className="mt-6 text-center">
                  <button
                    onClick={handleLoadMore}
                    disabled={loadingMore}
                    className="rounded-lg border border-gray-600 bg-gray-800 px-6 py-2 text-sm font-medium text-gray-200 transition-colors hover:bg-gray-700 disabled:opacity-50"
                  >
                    {loadingMore ? 'Loading...' : 'Load more'}
                  </button>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
