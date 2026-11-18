package com.jobs.im

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.size
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp

@Composable
fun ProfileAvatar(person: Person, model: ChatModel) {
    val bitmap by
        produceState<ImageBitmap?>(null, person.avatar, model.profile, model.userId, model.token) {
            value = null
            if (person.avatar.isNotBlank())
                runCatching {
                    model.image(person.avatar)?.let {
                        BitmapFactory.decodeByteArray(it, 0, it.size)?.asImageBitmap()
                    }
                }
                    .onSuccess { value = it }
        }
    Surface(shape = MaterialTheme.shapes.medium, modifier = Modifier.size(44.dp)) {
        if (bitmap != null) Image(bitmap!!, person.name, contentScale = ContentScale.Crop)
        else Image(painterResource(R.drawable.jobsim_logo), person.name)
    }
}
