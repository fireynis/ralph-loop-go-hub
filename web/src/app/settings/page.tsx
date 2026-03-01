'use client';

import { useEffect, useState } from 'react';

interface WebhookConfig {
  url: string;
  events: string[];
  filter: {
    passed_only: boolean;
  };
}

function maskUrl(url: string): string {
  if (url.length <= 30) return url;
  return url.slice(0, 30) + '...';
}

export default function SettingsPage() {
  const [webhooks, setWebhooks] = useState<WebhookConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // For now, show a placeholder since the API endpoint for settings
    // doesn't exist yet. This will be wired up later.
    setLoading(false);
  }, []);

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <div className="mx-auto max-w-4xl px-6 py-12">
        <h1 className="text-3xl font-bold tracking-tight text-white">
          Settings
        </h1>

        <section className="mt-10">
          <h2 className="text-xl font-semibold text-gray-200">Webhooks</h2>
          <p className="mt-1 text-sm text-gray-400">
            Webhook endpoints that receive notifications for pipeline events.
          </p>

          <div className="mt-6">
            {loading && (
              <p className="text-sm text-gray-400">Loading...</p>
            )}

            {error && (
              <div className="rounded-lg border border-red-800 bg-red-900/30 px-4 py-3">
                <p className="text-sm text-red-300">{error}</p>
              </div>
            )}

            {!loading && !error && webhooks.length === 0 && (
              <div className="rounded-lg border border-gray-700 bg-gray-800 px-6 py-8 text-center">
                <p className="text-sm text-gray-400">
                  No webhooks configured.
                </p>
              </div>
            )}

            {!loading && !error && webhooks.length > 0 && (
              <div className="space-y-4">
                {webhooks.map((webhook, index) => (
                  <div
                    key={index}
                    className="rounded-lg border border-gray-700 bg-gray-800 p-5"
                  >
                    <div className="space-y-3">
                      <div>
                        <span className="text-xs font-medium uppercase tracking-wide text-gray-500">
                          URL
                        </span>
                        <p className="mt-1 font-mono text-sm text-gray-300">
                          {maskUrl(webhook.url)}
                        </p>
                      </div>

                      <div>
                        <span className="text-xs font-medium uppercase tracking-wide text-gray-500">
                          Events
                        </span>
                        <div className="mt-1 flex flex-wrap gap-2">
                          {webhook.events.map((event) => (
                            <span
                              key={event}
                              className="inline-flex items-center rounded-full bg-gray-700 px-2.5 py-0.5 text-xs font-medium text-gray-200"
                            >
                              {event}
                            </span>
                          ))}
                        </div>
                      </div>

                      <div>
                        <span className="text-xs font-medium uppercase tracking-wide text-gray-500">
                          Passed only
                        </span>
                        <p className="mt-1 text-sm text-gray-300">
                          {webhook.filter.passed_only ? 'Yes' : 'No'}
                        </p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
