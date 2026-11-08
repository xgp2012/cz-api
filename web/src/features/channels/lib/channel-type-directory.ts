import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { getChannelTypeDirectory, type ChannelTypeDirectoryEntry } from '../api'
import { channelsQueryKeys } from './channel-actions'

export type { ChannelTypeDirectoryEntry }

/**
 * Reads the channel type directory the gateway publishes.
 *
 * The directory is static per build and per loaded plugin set, so it is cached
 * for the session instead of being refetched per form open. A failure is
 * reported without a global error redirect: the callers fall back to the
 * identifiers they already have from stored channels.
 */
export function useChannelTypeDirectory() {
  return useQuery({
    queryKey: channelsQueryKeys.typeDirectory(),
    queryFn: () => getChannelTypeDirectory().catch(() => null),
    // The picker hides a type's providers until its directory entry arrives, so
    // a refetch must keep the previous list on screen instead of blanking it.
    placeholderData: keepPreviousData,
    staleTime: 30 * 60 * 1000,
  })
}

export function isOfferableChannelType(
  entry: ChannelTypeDirectoryEntry
): boolean {
  return entry.configurable && !entry.deprecated
}
