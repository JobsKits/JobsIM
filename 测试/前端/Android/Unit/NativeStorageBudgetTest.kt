package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class NativeStorageBudgetTest {
    @Test fun aSmallBoundedWriteNeedsItsBytesAndReserveInsteadOfFourGiB() {
        assertTrue(NativeStorageBudget.fits(128L*1024*1024,16L*1024*1024))
        assertTrue(NativeStorageBudget.fits(NativeStorageBudget.reserve,0))
    }
    @Test fun insufficientSpaceIsRejectedBeforeConsumingTheRollbackReserve() {
        val bytes=1024L
        assertFalse(NativeStorageBudget.fits(NativeStorageBudget.reserve+bytes-1,bytes))
        assertTrue(NativeStorageBudget.fits(NativeStorageBudget.reserve+bytes,bytes))
        assertFalse(NativeStorageBudget.fits(0,0))
    }
    @Test fun negativeOverflowAndOversizeRequirementsCannotWrapIntoAnAllowedWrite() {
        for(value in listOf(-1L,Long.MAX_VALUE,NativeStorageBudget.maximumPending+1))assertTrue(runCatching{NativeStorageBudget.fits(Long.MAX_VALUE,value)}.isFailure)
        assertTrue(runCatching{NativeStorageBudget.fits(-1,0)}.isFailure)
        assertTrue(runCatching{NativeStorageBudget.total(listOf(NativeStorageBudget.maximumPending,1))}.isFailure)
    }
    @Test fun aBatchBudgetIncludesEveryBodyAndOriginalWithoutGrantingAnObjectSizeException() {
        val original=64L*1024*1024
        assertEquals(original+4096,NativeStorageBudget.total(listOf(original,2048,2048)))
        assertTrue(runCatching{NativeStorageBudget.total(listOf(0,-1))}.isFailure)
    }
}
