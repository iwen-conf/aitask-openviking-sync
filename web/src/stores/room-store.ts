import { create } from 'zustand'
import type { WsConnectionStatus } from '@/api/types'

interface RoomStore {
  status: WsConnectionStatus
  unreadMentions: number
  setStatus: (status: WsConnectionStatus) => void
  setUnreadMentions: (count: number) => void
}

export const useRoomStore = create<RoomStore>((set) => ({
  status: 'closed',
  unreadMentions: 0,
  setStatus: (status) => set({ status }),
  setUnreadMentions: (count) => set({ unreadMentions: count }),
}))
