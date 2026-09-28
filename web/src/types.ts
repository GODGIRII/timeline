export type Role = 'owner' | 'editor' | 'viewer';
export type Priority = 'low' | 'medium' | 'high';
export interface Account { id: string; username: string; display_name: string }
export interface Space { id: string; name: string; role: Role; key?: string }
export interface ItemFields {
  type: 'task' | 'event'; title: string; description: string;
  deadline: { date?: string; at?: string }; priority: Priority; status: 'open' | 'done';
}
export interface Item extends ItemFields {
  id: string; space_id: string; version: number; created_by: string; updated_by: string;
  created_at: string; updated_at: string; deleted: boolean;
}
export interface Activity {
  id: string; sequence: number; space_id: string; kind: string;
  actor: { id: string; display_name: string }; item: Item;
  message: string; created_at: string; changed_fields: string[];
}
export interface MemberData {
  members: { account: Account; role: Role }[];
  requests: { account: Account; status: string }[];
}
export type Connection = 'connecting' | 'live' | 'reconnecting' | 'denied';
export interface WriteCommand { path: string; method: 'POST' | 'PUT' | 'DELETE'; body: Record<string, unknown> }
