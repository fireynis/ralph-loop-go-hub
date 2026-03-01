'use client';

const PHASES = ['planner', 'dev', 'reviewer', 'fixer'];

interface PhaseIndicatorProps {
  currentPhase: string;
}

function getPhaseStatus(
  phaseIndex: number,
  currentIndex: number,
): 'completed' | 'current' | 'future' {
  if (currentIndex < 0) return 'future';
  if (phaseIndex < currentIndex) return 'completed';
  if (phaseIndex === currentIndex) return 'current';
  return 'future';
}

export function PhaseIndicator({ currentPhase }: PhaseIndicatorProps) {
  const currentIndex = PHASES.indexOf(currentPhase);

  return (
    <div className="flex items-center gap-1">
      {PHASES.map((phase, index) => {
        const status = getPhaseStatus(index, currentIndex);

        let dotColor: string;
        let textColor: string;
        if (status === 'completed') {
          dotColor = 'bg-emerald-500';
          textColor = 'text-emerald-400';
        } else if (status === 'current') {
          dotColor = 'bg-blue-500';
          textColor = 'text-blue-400 font-semibold';
        } else {
          dotColor = 'bg-gray-600';
          textColor = 'text-gray-500';
        }

        return (
          <div key={phase} className="flex items-center gap-1">
            {index > 0 && (
              <svg
                className={`h-4 w-4 ${
                  status === 'future' ? 'text-gray-600' : 'text-gray-400'
                }`}
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            )}
            <div className="flex items-center gap-1.5">
              <span className={`inline-block h-2.5 w-2.5 rounded-full ${dotColor}`} />
              <span className={`text-sm capitalize ${textColor}`}>{phase}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
