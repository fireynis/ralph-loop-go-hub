'use client';

interface IterationRecord {
  iteration: number;
  duration_ms: number;
  task_id: string;
  passed: boolean;
  notes: string;
  review_cycles: number;
  final_verdict: string;
  timestamp: string;
}

interface IterationTableProps {
  records: IterationRecord[];
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}m ${seconds}s`;
}

export type { IterationRecord };

export function IterationTable({ records }: IterationTableProps) {
  if (records.length === 0) {
    return (
      <div className="rounded-lg border border-gray-700 bg-gray-800 px-6 py-8 text-center">
        <p className="text-sm text-gray-400">No iteration history yet.</p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-lg border border-gray-700">
      <table className="w-full text-left text-sm">
        <thead className="border-b border-gray-700 bg-gray-800/80 text-xs uppercase tracking-wide text-gray-400">
          <tr>
            <th className="px-4 py-3">#</th>
            <th className="px-4 py-3">Task ID</th>
            <th className="px-4 py-3">Result</th>
            <th className="px-4 py-3">Duration</th>
            <th className="px-4 py-3">Reviews</th>
            <th className="px-4 py-3">Verdict</th>
            <th className="px-4 py-3">Notes</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-700">
          {records.map((record) => (
            <tr key={record.iteration} className="bg-gray-800 hover:bg-gray-750">
              <td className="px-4 py-3 font-medium text-gray-200">
                {record.iteration}
              </td>
              <td className="px-4 py-3 font-mono text-gray-300">
                {record.task_id}
              </td>
              <td className="px-4 py-3">
                {record.passed ? (
                  <span className="inline-flex items-center gap-1 text-emerald-400">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                    Pass
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-red-400">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                    </svg>
                    Fail
                  </span>
                )}
              </td>
              <td className="px-4 py-3 text-gray-300">
                {formatDuration(record.duration_ms)}
              </td>
              <td className="px-4 py-3 text-gray-300">{record.review_cycles}</td>
              <td className="px-4 py-3 text-gray-300">{record.final_verdict}</td>
              <td className="max-w-xs truncate px-4 py-3 text-gray-400" title={record.notes}>
                {record.notes}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
