// Created by Jobs on 2026年11月17日，星期二.
export class NativeStorageBudget {
  static reserve:number=64*1024*1024;
  static maximumPending:number=128*1024*1024+4096;
  static total(parts:number[]):number {let total=0;for(const size of parts){if(!Number.isSafeInteger(size)||size<0||size>NativeStorageBudget.maximumPending-total){throw new Error('STORAGE_BUDGET_INVALID');}total+=size;}return total;}
  static fits(available:number,required:number):boolean {if(!Number.isSafeInteger(available)||available<0||!Number.isSafeInteger(required)||required<0||required>NativeStorageBudget.maximumPending){throw new Error('STORAGE_BUDGET_INVALID');}return available>=NativeStorageBudget.reserve&&required<=available-NativeStorageBudget.reserve;}
}
