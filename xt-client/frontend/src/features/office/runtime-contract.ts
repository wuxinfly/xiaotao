import type {SceneMember} from './scene-contract';

export interface SceneRuntime {
  update(members: SceneMember[]): void;
  setActive(active: boolean): void;
  destroy(): void;
  pauseMember(id: string): void;
  zoomBy(amount: number): void;
  reset(): void;
  render(): void;
}
