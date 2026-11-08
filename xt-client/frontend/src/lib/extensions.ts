export interface MCPServer { command: string; args: string[]; disabled?: boolean }
export interface MCPSettings { mcpServers: Record<string, MCPServer>; source?: 'local' | 'environment' | 'empty' }
export interface SkillDirectory {path: string; disabled?: boolean}
export interface SkillsSettings {directories: SkillDirectory[]; disabledSkills: string[]}
export interface LocalSkill {id: string; name: string; description: string; path: string; directory: string; enabled: boolean; error?: string}
export interface SkillsCatalog {settings: SkillsSettings; skills: LocalSkill[]; issues: {path: string; message: string}[]; issuesOmitted?: number}
