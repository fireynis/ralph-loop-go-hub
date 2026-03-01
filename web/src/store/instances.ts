import { create } from 'zustand';
import type { InstanceState, RalphEvent } from '@/lib/types';

const MAX_RECENT_EVENTS = 100;

interface InstanceStore {
  instances: Map<string, InstanceState>;
  recentEvents: RalphEvent[];
  setInstances: (instances: InstanceState[]) => void;
  handleEvent: (event: RalphEvent) => void;
}

export const useInstanceStore = create<InstanceStore>((set) => ({
  instances: new Map(),
  recentEvents: [],

  setInstances: (instances: InstanceState[]) => {
    const map = new Map<string, InstanceState>();
    for (const inst of instances) {
      map.set(inst.instance_id, inst);
    }
    set({ instances: map });
  },

  handleEvent: (event: RalphEvent) => {
    set((state) => {
      // Add event to recent events list (newest first, capped at 100)
      const recentEvents = [event, ...state.recentEvents].slice(
        0,
        MAX_RECENT_EVENTS,
      );

      // Update the instance map
      const instances = new Map(state.instances);
      const existing = instances.get(event.instance_id);

      let status = existing?.status ?? 'unknown';
      if (event.type === 'session.started') {
        status = 'running';
      } else if (event.type === 'session.ended') {
        status = 'ended';
      } else if (event.context?.status) {
        status = event.context.status;
      }

      instances.set(event.instance_id, {
        instance_id: event.instance_id,
        repo: event.repo,
        epic: event.epic || existing?.epic,
        status,
        last_event: event.timestamp,
        context: event.context ?? existing?.context,
      });

      return { instances, recentEvents };
    });
  },
}));
