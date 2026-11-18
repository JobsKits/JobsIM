import UIAbility from '@ohos.app.ability.UIAbility';
import window from '@ohos.window';
export default class EntryAbility extends UIAbility {
  onForeground(): void { AppStorage.setOrCreate<boolean>('JobsIM.foreground', true); }
  onBackground(): void { AppStorage.setOrCreate<boolean>('JobsIM.foreground', false); }
  onWindowStageCreate(stage: window.WindowStage): void {
    stage.loadContent('pages/Index');
  }
}
