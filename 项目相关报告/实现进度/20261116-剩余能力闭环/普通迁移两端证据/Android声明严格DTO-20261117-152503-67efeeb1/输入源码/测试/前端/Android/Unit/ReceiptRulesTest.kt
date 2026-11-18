package com.jobs.im

import java.util.UUID
import org.junit.Assert.*
import org.junit.Test

class ReceiptRulesTest {
    private val original=mapOf<String,Any?>("storeGeneration" to UUID.randomUUID().toString(),"profileId" to "native-approved","buildDigest" to "c".repeat(64),"verificationDigest" to "d".repeat(64))
    private fun fact(contract:String=ReceiptProtocol.objectContract):Map<String,Any?> = original+mapOf("restoreEpoch" to "1","qualificationId" to UUID.randomUUID().toString(),"storageContract" to contract)
    private fun checked(value:Map<String,Any?>)=ReceiptProtocol.storageFact(value,original,"1")

    @Test fun legacySevenAndPairedNineBecomeTheSameImmutableSevenFact() {
        for(contract in listOf(ReceiptProtocol.contentContract,ReceiptProtocol.objectContract)) {
            val old=fact(contract)
            val current=old+mapOf("qualified" to true,"objectQualified" to (contract==ReceiptProtocol.objectContract))
            assertEquals(old,checked(old));assertEquals(checked(old),checked(current));assertEquals(7,checked(current).size)
            assertFalse(checked(current).containsKey("qualified"));assertFalse(checked(current).containsKey("objectQualified"))
        }
    }
    @Test fun incompleteFlagsNonBooleansFalseQualificationAndContractMismatchAreRejected() {
        for(contract in listOf(ReceiptProtocol.contentContract,ReceiptProtocol.objectContract)) {
            val base=fact(contract);val objects=contract==ReceiptProtocol.objectContract
            val invalid=listOf(base+mapOf("qualified" to true),base+mapOf("objectQualified" to objects),base+mapOf("qualified" to "true","objectQualified" to objects),base+mapOf("qualified" to true,"objectQualified" to "false"),base+mapOf("qualified" to false,"objectQualified" to objects),base+mapOf("qualified" to true,"objectQualified" to !objects),base+mapOf("qualified" to null,"objectQualified" to objects))
            for(value in invalid)assertTrue(runCatching{checked(value)}.isFailure)
        }
    }
    @Test fun requiredMetadataCannotBeMissingRetypedOrExtended() {
        val base=fact()
        for(key in base.keys) {
            assertTrue(runCatching{checked(base-key)}.isFailure)
            assertTrue(runCatching{checked(base+mapOf(key to 1))}.isFailure)
        }
        assertTrue(runCatching{checked(base+mapOf("unexpected" to true))}.isFailure)
        assertTrue(runCatching{checked(base+mapOf("storageContract" to "foreign"))}.isFailure)
    }
    @Test fun originalTupleAndQualificationStayBoundToTheStoredDeclaration() {
        val base=fact()
        for(key in original.keys)assertTrue(runCatching{checked(base+mapOf(key to "substituted"))}.isFailure)
        assertTrue(runCatching{checked(base+mapOf("restoreEpoch" to "2"))}.isFailure)
        assertTrue(runCatching{checked(base+mapOf("qualificationId" to "invalid"))}.isFailure)
    }
}
