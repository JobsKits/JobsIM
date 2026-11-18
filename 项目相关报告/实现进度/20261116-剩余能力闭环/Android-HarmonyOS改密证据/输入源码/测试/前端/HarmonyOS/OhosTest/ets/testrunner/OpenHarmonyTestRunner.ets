import TestRunner from '@ohos.application.testRunner';
import AbilityDelegatorRegistry from '@ohos.app.ability.abilityDelegatorRegistry';
import { Hypium } from '@ohos/hypium';
import testsuite from '../test/List.test';
export default class OpenHarmonyTestRunner implements TestRunner {
  onPrepare(): void {}
  onRun(): void {
    Hypium.hypiumTest(AbilityDelegatorRegistry.getAbilityDelegator(), AbilityDelegatorRegistry.getArguments(), testsuite);
  }
}
