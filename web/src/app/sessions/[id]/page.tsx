'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import type { Session, RalphEvent } from '@/lib/types';

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

type BadgeStyle = { bg: string; text: string };

function eventBadgeStyle(eventType: string, data?: { passed?: boolean }): BadgeStyle {
  if (eventType === 'session.started' || eventType === 'session.ended') {
    return { bg: 'bg-blue-900/50 border-blue-700', text: 'text-blue-300' };
  }
  if (eventType === 'iteration.completed') {
    if (data?.passed) {
      return { bg: 'bg-green-900/50 border-green-700', text: 'text-green-300' };
    }
    return { bg: 'bg-red-900/50 border-red-700', text: 'text-red-300' };
  }
  if (eventType === 'phase.changed') {
    return { bg: 'bg-purple-900/50 border-purple-700', text: 'text-purple-300' };
  }
  if (eventType === 'task.claimed' || eventType === 'task.closed') {
    return { bg: 'bg-amber-900/50 border-amber-700', text: 'text-amber-300' };
  }
  return { bg: 'bg-gray-700/50 border-gray-600', text: 'text-gray-300' };
}

function eventSummary(event: RalphEvent): string | null {
  const d = event.data;
  if (!d) return null;

  switch (event.type) {
    case 'session.started':
      return d.max_iterations != null
        ? `Max iterations: ${d.max_iterations}`
        : null;
    case 'session.ended':
      return d.reason || null;
    case 'iteration.completed':
      return [
        d.iteration != null ? `Iteration #${d.iteration}` : null,
        d.passed != null ? (d.passed ? 'Passed' : 'Failed') : null,
        d.duration_ms != null ? `${(d.duration_ms / 1000).toFixed(1)}s` : null,
        d.review_cycles != null ? `${d.review_cycles} review cycle(s)` : null,
      ]
        .filter(Boolean)
        .join(' \u00b7 ');
    case 'phase.changed':
      if (d.from && d.to) {
        return `${d.from} \u2192 ${d.to}`;
      }
      return d.phase || null;
    case 'task.claimed':
      return [
        d.task_id ? `Task: ${d.task_id}` : null,
        d.description || null,
      ]
        .filter(Boolean)
        .join(' \u2014 ');
    case 'task.closed':
      return [
        d.task_id ? `Task: ${d.task_id}` : null,
        d.final_verdict || null,
        d.commit_hash ? `commit ${d.commit_hash.slice(0, 8)}` : null,
      ]
        .filter(Boolean)
        .join(' \u00b7 ');
    default:
      return d.notes || d.description || d.reason || null;
  }
}

interface SessionDetailResponse {
  session: Session;
  events: RalphEvent[];
}

export default function SessionDetailPage() {
  const params = useParams<{ id: string }>();
  const [session, setSession] = useState<Session | null>(null);
  const [events, setEvents] = useState<RalphEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!params.id) return;

    async function fetchSession() {
      try {
        const res = await fetch(
          `${getApiBase()}/api/v1/sessions/${params.id}`
        );
        if (!res.ok) throw new Error(`API error: ${res.status}`);
        const data: SessionDetailResponse = await res.json();
        setSession(data.session);
        setEvents(data.events);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to fetch session'
        );
      } finally {
        setLoading(false);
      }
    }

    fetchSession();
  }, [params.id]);

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-900 text-gray-100">
        <div className="mx-auto max-w-4xl px-6 py-12">
          <p className="text-sm text-gray-400">Loading...</p>
        </div>
      </div>
    );
  }

  if (error || !session) {
    return (
      <div className="min-h-screen bg-gray-900 text-gray-100">
        <div className="mx-auto max-w-4xl px-6 py-12">
          <div className="rounded-lg border border-red-800 bg-red-900/30 px-4 py-3">
            <p className="text-sm text-red-300">
              {error || 'Session not found'}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <div className="mx-auto max-w-4xl px-6 py-12">
        {/* Header */}
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-white">
            {session.repo}
          </h1>
          <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-gray-400">
            {session.epic && (
              <span className="rounded-full bg-gray-800 px-3 py-0.5 text-gray-300">
                {session.epic}
              </span>
            )}
            <span className="font-mono text-xs text-gray-500">
              {session.session_id}
            </span>
          </div>
        </div>

        {/* Stats Summary */}
        <div className="mt-8 grid grid-cols-2 gap-4 sm:grid-cols-5">
          <div className="rounded-lg border border-gray-700 bg-gray-800 px-4 py-3">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Started
            </p>
            <p className="mt-1 text-sm text-gray-200">
              {formatDate(session.started_at)}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 px-4 py-3">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Ended
            </p>
            <p className="mt-1 text-sm">
              {session.ended_at ? (
                <span className="text-gray-200">
                  {formatDate(session.ended_at)}
                </span>
              ) : (
                <span className="inline-flex items-center gap-1.5 text-green-400">
                  <span className="inline-block h-2 w-2 rounded-full bg-green-400" />
                  Running
                </span>
              )}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 px-4 py-3">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Iterations
            </p>
            <p className="mt-1 text-lg font-semibold tabular-nums text-gray-200">
              {session.iterations}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 px-4 py-3">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Tasks Closed
            </p>
            <p className="mt-1 text-lg font-semibold tabular-nums text-gray-200">
              {session.tasks_closed}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 px-4 py-3">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Pass Rate
            </p>
            <p
              className={`mt-1 text-lg font-semibold tabular-nums ${passRateColor(session.pass_rate)}`}
            >
              {(session.pass_rate * 100).toFixed(0)}%
            </p>
          </div>
        </div>

        {/* Event Timeline */}
        <section className="mt-10">
          <h2 className="text-xl font-semibold text-gray-200">
            Event Timeline
          </h2>
          <p className="mt-1 text-sm text-gray-400">
            {events.length} event{events.length !== 1 ? 's' : ''}
          </p>

          {events.length === 0 ? (
            <div className="mt-6 rounded-lg border border-gray-700 bg-gray-800 px-6 py-8 text-center">
              <p className="text-sm text-gray-400">
                No events recorded for this session.
              </p>
            </div>
          ) : (
            <div className="mt-6 space-y-0">
              {events.map((event, idx) => {
                const badge = eventBadgeStyle(event.type, event.data);
                const summary = eventSummary(event);
                return (
                  <div key={event.event_id} className="relative flex gap-4">
                    {/* Timeline line */}
                    <div className="flex w-6 flex-col items-center">
                      <div
                        className={`mt-2 h-3 w-3 rounded-full border ${badge.bg} shrink-0`}
                      />
                      {idx < events.length - 1 && (
                        <div className="w-px flex-1 bg-gray-700" />
                      )}
                    </div>

                    {/* Event content */}
                    <div className="pb-6">
                      <div className="flex flex-wrap items-center gap-2">
                        <span
                          className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${badge.bg} ${badge.text}`}
                        >
                          {event.type}
                        </span>
                        <span className="text-xs text-gray-500">
                          {formatDate(event.timestamp)}
                        </span>
                      </div>
                      {summary && (
                        <p className="mt-1 text-sm text-gray-300">{summary}</p>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
