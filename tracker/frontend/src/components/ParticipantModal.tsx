import { useEffect } from 'react'
import { Participant } from '../types/inference'

interface ParticipantModalProps {
  participant: Participant | null
  onClose: () => void
}

export function ParticipantModal({ participant, onClose }: ParticipantModalProps) {
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
      }
    }

    if (participant) {
      document.addEventListener('keydown', handleEscape)
    }

    return () => {
      document.removeEventListener('keydown', handleEscape)
    }
  }, [participant, onClose])

  if (!participant) {
    return null
  }

  const totalInferenced = parseInt(participant.current_epoch_stats.inference_count) + 
                         parseInt(participant.current_epoch_stats.missed_requests)

  return (
    <div 
      className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4"
      onClick={onClose}
    >
      <div 
        className="bg-white rounded-lg shadow-xl max-w-3xl w-full max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sticky top-0 bg-white border-b border-gray-200 px-6 py-4 flex justify-between items-center">
          <h2 className="text-xl font-semibold text-gray-900">Participant Details</h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 text-2xl leading-none"
          >
            &times;
          </button>
        </div>

        <div className="px-6 py-4 space-y-6">
          <div className="space-y-4">
            <div>
              <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Participant Address</label>
              <div className="mt-1 text-sm font-mono text-gray-900 break-all">{participant.index}</div>
            </div>

            <div>
              <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Consensus Key</label>
              <div className="mt-1 text-sm font-mono text-gray-900 break-all">
                {participant.validator_key || <span className="text-gray-400">Not available</span>}
              </div>
            </div>

            <div>
              <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">URL</label>
              <div className="mt-1 text-sm text-gray-900 break-all">
                {participant.inference_url ? (
                  <a href={participant.inference_url} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                    {participant.inference_url}
                  </a>
                ) : (
                  <span className="text-gray-400">Not available</span>
                )}
              </div>
            </div>

            <div className="flex gap-8">
              <div>
                <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Weight</label>
                <div className="mt-1 text-sm font-semibold text-gray-900">{participant.weight.toLocaleString()}</div>
              </div>

              <div>
                <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Jail Status</label>
                <div className="mt-1">
                  {participant.is_jailed === true ? (
                    <span className="inline-block px-2 py-0.5 text-xs font-semibold bg-red-100 text-red-700 border border-red-300 rounded">
                      JAILED
                    </span>
                  ) : participant.is_jailed === false ? (
                    <span className="inline-block px-2 py-0.5 text-xs font-semibold bg-green-100 text-green-700 border border-green-300 rounded">
                      ACTIVE
                    </span>
                  ) : (
                    <span className="text-gray-400 text-xs">Unknown</span>
                  )}
                </div>
              </div>

              <div>
                <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Health Status</label>
                <div className="mt-1 flex items-center gap-2">
                  {participant.node_healthy === true ? (
                    <>
                      <div className="w-3 h-3 bg-green-500 rounded-full"></div>
                      <span className="text-sm text-gray-900">Healthy</span>
                    </>
                  ) : participant.node_healthy === false ? (
                    <>
                      <div className="w-3 h-3 bg-red-500 rounded-full"></div>
                      <span className="text-sm text-gray-900">Unhealthy</span>
                    </>
                  ) : (
                    <>
                      <div className="w-3 h-3 bg-gray-300 rounded-full"></div>
                      <span className="text-sm text-gray-400">Unknown</span>
                    </>
                  )}
                </div>
              </div>
            </div>

            <div>
              <label className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Models</label>
              <div className="mt-2 flex flex-wrap gap-2">
                {participant.models.length > 0 ? (
                  participant.models.map((model, idx) => (
                    <span
                      key={idx}
                      className="inline-block px-2 py-1 text-xs font-medium bg-gray-100 text-gray-700 border border-gray-300 rounded"
                    >
                      {model}
                    </span>
                  ))
                ) : (
                  <span className="text-gray-400 text-sm">No models</span>
                )}
              </div>
            </div>
          </div>

          <div className="border-t border-gray-200 pt-6">
            <h3 className="text-sm font-semibold text-gray-700 uppercase tracking-wider mb-4">Inference Statistics</h3>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="bg-gray-50 p-4 rounded">
                <div className="text-xs text-gray-500 uppercase tracking-wider">Total Inferenced</div>
                <div className="mt-1 text-2xl font-semibold text-gray-900">{totalInferenced.toLocaleString()}</div>
              </div>

              <div className="bg-gray-50 p-4 rounded">
                <div className="text-xs text-gray-500 uppercase tracking-wider">Missed Requests</div>
                <div className={`mt-1 text-2xl font-semibold ${parseInt(participant.current_epoch_stats.missed_requests) > 0 ? 'text-red-600' : 'text-gray-900'}`}>
                  {parseInt(participant.current_epoch_stats.missed_requests).toLocaleString()}
                </div>
              </div>

              <div className="bg-gray-50 p-4 rounded">
                <div className="text-xs text-gray-500 uppercase tracking-wider">Validated Inferences</div>
                <div className="mt-1 text-2xl font-semibold text-gray-900">
                  {parseInt(participant.current_epoch_stats.validated_inferences).toLocaleString()}
                </div>
              </div>

              <div className="bg-gray-50 p-4 rounded">
                <div className="text-xs text-gray-500 uppercase tracking-wider">Invalidated Inferences</div>
                <div className={`mt-1 text-2xl font-semibold ${parseInt(participant.current_epoch_stats.invalidated_inferences) > 0 ? 'text-red-600' : 'text-gray-900'}`}>
                  {parseInt(participant.current_epoch_stats.invalidated_inferences).toLocaleString()}
                </div>
              </div>

              <div className="bg-gray-50 p-4 rounded">
                <div className="text-xs text-gray-500 uppercase tracking-wider">Missed Rate</div>
                <div className={`mt-1 text-2xl font-semibold ${participant.missed_rate > 0.10 ? 'text-red-600' : 'text-gray-900'}`}>
                  {(participant.missed_rate * 100).toFixed(2)}%
                </div>
              </div>

              <div className="bg-gray-50 p-4 rounded">
                <div className="text-xs text-gray-500 uppercase tracking-wider">Invalidation Rate</div>
                <div className={`mt-1 text-2xl font-semibold ${participant.invalidation_rate > 0.10 ? 'text-red-600' : 'text-gray-900'}`}>
                  {(participant.invalidation_rate * 100).toFixed(2)}%
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

