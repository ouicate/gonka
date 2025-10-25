package com.productscience.assertions

import com.productscience.data.TxResponse
import org.assertj.core.api.AbstractAssert

/**
 * Custom AssertJ assertion for [TxResponse].
 *
 * Usage examples:
 *  - import com.productscience.assertions.assertThat
 *    assertThat(txResponse).IsSuccess()
 */
class TxResponseAssert(actual: TxResponse) :
    AbstractAssert<TxResponseAssert, TxResponse>(actual, TxResponseAssert::class.java) {

    /**
     * Verifies that the transaction was successful (i.e., result code == 0).
     */
    fun isSuccess(): TxResponseAssert {
        isNotNull()
        if (actual.code != 0) {
            failWithMessage(
                "Expected transaction to succeed (code=0) but was %s. txhash=%s, rawLog=%s",
                actual.code,
                actual.txhash,
                actual.rawLog
            )
        }
        return this
    }

    fun isFailure(): TxResponseAssert {
        isNotNull()
        if (actual.code == 0) {
            failWithMessage(
                "Expected transaction to fail (code!=0) but was %s. txhash=%s, rawLog=%s",
                actual.code,
                actual.txhash,
                actual.rawLog
            )
        }
        return this
    }
}

/** Factory function to start assertions for [TxResponse]. */
fun assertThat(actual: TxResponse): TxResponseAssert = TxResponseAssert(actual)
