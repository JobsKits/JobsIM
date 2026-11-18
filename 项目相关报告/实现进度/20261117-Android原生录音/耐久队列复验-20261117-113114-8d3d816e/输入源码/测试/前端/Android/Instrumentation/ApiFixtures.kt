package com.jobs.im

import org.json.JSONObject

suspend fun conversationSnapshot(api: Api, id: String): JSONObject {
    val items = api.request("/conversations").getJSONArray("items")
    return (0 until items.length())
        .map { items.getJSONObject(it) }
        .single { it.getString("id") == id }
}
