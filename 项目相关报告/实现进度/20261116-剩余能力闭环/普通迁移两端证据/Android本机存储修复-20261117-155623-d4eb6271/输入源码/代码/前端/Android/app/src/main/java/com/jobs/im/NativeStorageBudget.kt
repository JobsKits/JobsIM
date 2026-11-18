package com.jobs.im

object NativeStorageBudget {
    const val reserve=64L*1024*1024
    const val maximumPending=128L*1024*1024+4096
    fun total(parts:List<Long>):Long {
        var result=0L
        for(size in parts) {require(size>=0&&size<=maximumPending-result);result+=size}
        return result
    }
    fun fits(available:Long,required:Long):Boolean {
        require(available>=0&&required>=0&&required<=maximumPending)
        return available>=reserve&&required<=available-reserve
    }
}
