


// fetch("https://canvas.instructure.com/api/v1/users/self/favorites/courses?include[]=total_scores", {
//     headers: {
//         "Authorization": "Bearer 11299~RXwwAUU7Px2BBLQ6YUcY6zGP72Fu3GuRehHLTB8AEZNWyFrU9XFBJkmJMwuK4ZCu"
//     }
// })
// .then(response => response.json())
// .then(data => console.log(data))
// .catch(error => console.error('Error:', error));



fetch("https://csus.instructure.com/api/v1/courses/137363/assignments", {
    headers: {
        "Authorization": "Bearer 11299~RXwwAUU7Px2BBLQ6YUcY6zGP72Fu3GuRehHLTB8AEZNWyFrU9XFBJkmJMwuK4ZCu"
    }
})
.then(response => response.json())
.then(data => console.log(data))
.catch(error => console.error('Error:', error));