'use client';

import { useInstanceStore } from '@/store/instances';
import { InstanceCard } from '@/components/instance-card';

export default function OverviewPage() {
  const instances = useInstanceStore((s) => s.instances);

  const allInstances = Array.from(instances.values());
  const active = allInstances.filter((i) => i.status === 'running');
  const inactive = allInstances.filter((i) => i.status !== 'running');

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Overview</h1>

      {allInstances.length === 0 && (
        <div className="text-gray-500 text-center py-12">
          No Ralph instances connected yet. Start a Ralph loop with -hub-url
          pointing here.
        </div>
      )}

      {active.length > 0 && (
        <section className="mb-8">
          <h2 className="text-lg font-semibold text-gray-400 mb-4">
            Active ({active.length})
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {active.map((inst) => (
              <InstanceCard key={inst.instance_id} instance={inst} />
            ))}
          </div>
        </section>
      )}

      {inactive.length > 0 && (
        <section>
          <h2 className="text-lg font-semibold text-gray-400 mb-4">
            Inactive ({inactive.length})
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {inactive.map((inst) => (
              <InstanceCard key={inst.instance_id} instance={inst} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
