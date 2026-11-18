package com.jobs.im

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

data class ReceiptCandidate(val id:String,val version:String)
class ReceiptCoordinator(private val scope:CoroutineScope,private val enabled:()->Boolean,private val factory:suspend(()->Boolean)->ReceiptService,private val cached:suspend(String,Int)->List<ReceiptCandidate>) {
    private val pending=linkedMapOf<String,ReceiptCandidate>()
    private var cursor=""
    private var revision=0
    private var work:Job?=null
    fun stop(){revision++;work?.cancel();work=null;pending.clear();cursor=""}
    fun offer(rows:List<ReceiptCandidate>){
        if(!enabled())return
        rows.forEach{pending[it.id]=it};while(pending.size>512)pending.remove(pending.keys.first())
        if(work?.isActive==true)return
        val stamp=revision
        work=scope.launch{
            var service:ReceiptService?=null
            val current={revision==stamp&&enabled()}
            try{
                if(!current())return@launch;val active=factory(current);service=active;active.reconcile();if(!current())return@launch
                val page=cached(cursor,8);if(!current())return@launch;cursor=page.lastOrNull()?.id.orEmpty();for(row in page)pending.putIfAbsent(row.id,row)
                for(row in pending.values.take(8).toList()){
                    if(!current())break
                    try{active.receiveIfTarget(row.id,row.version)}catch(_:Exception){if(!current())break}
                    pending.remove(row.id)
                }
            }catch(_:Exception){ /* Original responsibilities stay durable; the next sync only checks their results. */ }
            finally{service?.clear();if(revision==stamp)work=null}
        }
    }
}
