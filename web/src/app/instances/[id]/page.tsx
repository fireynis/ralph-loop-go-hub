'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import Link from 'next/link';
import { PhaseIndicator } from '@/components/phase-indicator';
import { IterationTable } from '@/components/iteration-table';
import type { IterationRecord } from '@/components/iteration-table';
import { useInstanceStore } from '@/store/instances';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';

function getApiBaseUrl(): string {
  if (typeof window === 'undefined') return '';
  return window.location.origin;
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}m ${seconds}s`;
}

function getStatusBadge(status: string) {
  const styles: Record<string, string> = {
    running: 'bg-emerald-900/50 text-emerald-300 border-emerald-700',
    ended: 'bg-gray-700/50 text-gray-300 border-gray-600',
    unknown: 'bg-yellow-900/50 text-yellow-300 border-yellow-700',
  };

  const style = styles[status] ?? styles.unknown;

  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${style}`}
    >
      {status}
    </span>
  );
}

interface PassRateDataPoint {
  iteration: number;
  passRate: number;
}

interface DurationDataPoint {
  iteration: number;
  durationSeconds: number;
}

function computePassRateData(records: IterationRecord[]): PassRateDataPoint[] {
  const sorted = [...records].sort((a, b) => a.iteration - b.iteration);
  let passed = 0;
  let total = 0;

  return sorted.map((r) => {
    total += 1;
    if (r.passed) passed += 1;
    return {
      iteration: r.iteration,
      passRate: Math.round((passed / total) * 100),
    };
  });
}

function computeDurationData(records: IterationRecord[]): DurationDataPoint[] {
  const sorted = [...records].sort((a, b) => a.iteration - b.iteration);
  return sorted.map((r) => ({
    iteration: r.iteration,
    durationSeconds: Math.round(r.duration_ms / 1000),
  }));
}

export default function InstanceDetailPage() {
  const params = useParams();
  const instanceId = decodeURIComponent(params.id as string);

  const instance = useInstanceStore((state) => state.instances.get(instanceId));

  const [records, setRecords] = useState<IterationRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchHistory() {
      try {
        const baseUrl = getApiBaseUrl();
        const res = await fetch(
          `${baseUrl}/api/v1/instances/${encodeURIComponent(instanceId)}/history`,
        );
        if (!res.ok) {
          throw new Error(`Failed to fetch history: ${res.status}`);
        }
        const data = await res.json();
        setRecords(data ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load history');
      } finally {
        setLoading(false);
      }
    }

    fetchHistory();
  }, [instanceId]);

  const analytics = instance?.context?.analytics;
  const passRateData = computePassRateData(records);
  const durationData = computeDurationData(records);

  const passRate =
    analytics && analytics.passed_count + analytics.failed_count > 0
      ? Math.round(
          (analytics.passed_count /
            (analytics.passed_count + analytics.failed_count)) *
            100,
        )
      : null;

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <div className="mx-auto max-w-6xl px-6 py-8">
        {/* Back link */}
        <Link
          href="/"
          className="mb-6 inline-flex items-center gap-1 text-sm text-gray-400 hover:text-gray-200 transition-colors"
        >
          <svg
            className="h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M15 19l-7-7 7-7"
            />
          </svg>
          Back to instances
        </Link>

        {/* Instance header */}
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <h1 className="text-3xl font-bold tracking-tight text-white">
              {instance?.repo ?? instanceId}
            </h1>
            {instance && getStatusBadge(instance.status)}
          </div>
          {instance?.epic && (
            <p className="text-gray-400 text-lg">{instance.epic}</p>
          )}
        </div>

        {/* Phase Indicator */}
        <section className="mb-8">
          <h2 className="text-sm font-medium uppercase tracking-wide text-gray-500 mb-3">
            Pipeline Phase
          </h2>
          <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
            <PhaseIndicator
              currentPhase={instance?.context?.current_phase ?? ''}
            />
          </div>
        </section>

        {/* Stats row */}
        <section className="mb-8 grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Iterations
            </p>
            <p className="mt-1 text-2xl font-bold text-white">
              {instance?.context?.current_iteration ?? records.length}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Pass Rate
            </p>
            <p
              className={`mt-1 text-2xl font-bold ${
                passRate !== null && passRate >= 70
                  ? 'text-emerald-400'
                  : passRate !== null
                    ? 'text-amber-400'
                    : 'text-gray-400'
              }`}
            >
              {passRate !== null ? `${passRate}%` : '--'}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Tasks Closed
            </p>
            <p className="mt-1 text-2xl font-bold text-white">
              {analytics?.tasks_closed ?? 0}
            </p>
          </div>
          <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              Avg Duration
            </p>
            <p className="mt-1 text-2xl font-bold text-white">
              {analytics?.avg_duration_ms
                ? formatDuration(analytics.avg_duration_ms)
                : '--'}
            </p>
          </div>
        </section>

        {/* Charts */}
        {loading && (
          <p className="text-sm text-gray-400 mb-8">Loading history...</p>
        )}

        {error && (
          <div className="mb-8 rounded-lg border border-red-800 bg-red-900/30 px-4 py-3">
            <p className="text-sm text-red-300">{error}</p>
          </div>
        )}

        {!loading && !error && records.length > 0 && (
          <section className="mb-8 grid gap-6 lg:grid-cols-2">
            {/* Pass rate over time */}
            <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
              <h3 className="mb-4 text-sm font-medium uppercase tracking-wide text-gray-400">
                Pass Rate Over Time
              </h3>
              <ResponsiveContainer width="100%" height={250}>
                <LineChart data={passRateData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                  <XAxis
                    dataKey="iteration"
                    stroke="#6B7280"
                    tick={{ fill: '#9CA3AF', fontSize: 12 }}
                    label={{
                      value: 'Iteration',
                      position: 'insideBottom',
                      offset: -5,
                      fill: '#9CA3AF',
                      fontSize: 12,
                    }}
                  />
                  <YAxis
                    stroke="#6B7280"
                    tick={{ fill: '#9CA3AF', fontSize: 12 }}
                    domain={[0, 100]}
                    label={{
                      value: '%',
                      position: 'insideLeft',
                      fill: '#9CA3AF',
                      fontSize: 12,
                    }}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: '#1F2937',
                      border: '1px solid #374151',
                      borderRadius: '0.5rem',
                      color: '#F3F4F6',
                    }}
                    formatter={(value: number | undefined) => [
                      `${value ?? 0}%`,
                      'Pass Rate',
                    ]}
                  />
                  <Line
                    type="monotone"
                    dataKey="passRate"
                    stroke="#10B981"
                    strokeWidth={2}
                    dot={{ fill: '#10B981', r: 3 }}
                    activeDot={{ r: 5 }}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>

            {/* Duration trend */}
            <div className="rounded-lg border border-gray-700 bg-gray-800 p-4">
              <h3 className="mb-4 text-sm font-medium uppercase tracking-wide text-gray-400">
                Duration Trend
              </h3>
              <ResponsiveContainer width="100%" height={250}>
                <LineChart data={durationData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                  <XAxis
                    dataKey="iteration"
                    stroke="#6B7280"
                    tick={{ fill: '#9CA3AF', fontSize: 12 }}
                    label={{
                      value: 'Iteration',
                      position: 'insideBottom',
                      offset: -5,
                      fill: '#9CA3AF',
                      fontSize: 12,
                    }}
                  />
                  <YAxis
                    stroke="#6B7280"
                    tick={{ fill: '#9CA3AF', fontSize: 12 }}
                    label={{
                      value: 'Seconds',
                      position: 'insideLeft',
                      fill: '#9CA3AF',
                      fontSize: 12,
                      angle: -90,
                    }}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: '#1F2937',
                      border: '1px solid #374151',
                      borderRadius: '0.5rem',
                      color: '#F3F4F6',
                    }}
                    formatter={(value: number | undefined) => [
                      `${value ?? 0}s`,
                      'Duration',
                    ]}
                  />
                  <Line
                    type="monotone"
                    dataKey="durationSeconds"
                    stroke="#3B82F6"
                    strokeWidth={2}
                    dot={{ fill: '#3B82F6', r: 3 }}
                    activeDot={{ r: 5 }}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </section>
        )}

        {/* Iteration Table */}
        <section>
          <h2 className="mb-4 text-sm font-medium uppercase tracking-wide text-gray-500">
            Iteration History
          </h2>
          {loading ? (
            <p className="text-sm text-gray-400">Loading...</p>
          ) : error ? null : (
            <IterationTable records={records} />
          )}
        </section>
      </div>
    </div>
  );
}
