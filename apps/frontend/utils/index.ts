export function parseTitle(title: string) {
	const r = new RegExp(/::(year|month|day):(\+|-)?(\d+)?:/, 'g')
	let m
	let t = title
	while ((m = r.exec(title)) !== null) {
		let now = new Date()
		let val = 0
		if (m[3]) {
			try {
				val = parseInt(m[3])
			} catch (e) {
				val = 0
			}
		}
		if (m[2] === '-') {
			val = -val
		}
		if (m[1] === 'year') {
			t = t.replace(m[0], (now.getFullYear() + val).toString())
		} else if (m[1] === 'month') {
			t = t.replace(m[0], (now.getMonth() + val).toString())
		} else if (m[1] === 'day') {
			t = t.replace(m[0], (now.getDate() + val).toString())
		}
	}
	return t
}
