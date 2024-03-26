export const stripePrefix = (pk: string) => (pk.slice(pk.indexOf(':') + 1))

export const getFlagEmoji = (country: string) => {
	if (country === '') return 'N/A'
	const codePoints = country
	.toUpperCase()
	.split('')
	.map(char => 127397 + char.charCodeAt(0))
	return country + ' ' + String.fromCodePoint(...codePoints)
}

export const blocksToTime = (blocks: number) => {
	if (blocks < 144 * 7) return (blocks / 144).toFixed(1) + ' days'
	if (blocks < 144 * 30) return (blocks / 144 / 7).toFixed(1) + ' weeks'
	return (blocks / 144 / 30).toFixed(1) + ' months'
}

export const convertSize = (size: number) => {
	if (size < 1024) {
		return '' + size + ' B'
	}
	const sizes = ['KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']
	let i = Math.floor(Math.log10(size) / 3)
	let s = (size / Math.pow(10, 3 * i)).toFixed(1)
	return s.slice(0, s.indexOf('.') + i + 1) + ' ' + sizes[i - 1]
}

export const convertPrice = (value: string) => {
	if (value.length < 13) return value + ' H'
	if (value.length < 16) return value.slice(0, value.length - 12) + ' pS'
	if (value.length < 19) return value.slice(0, value.length - 15) + ' nS'
	if (value.length < 22) return value.slice(0, value.length - 18) + ' uS'
	if (value.length < 25) return value.slice(0, value.length - 21) + ' mS'
	if (value.length < 28) {
		let result = value.slice(0, value.length - 24)
		if (value[value.length-24] !== '0') result += '.' + value[value.length-24]
		return result + ' SC'
	}
	if (value.length < 31) {
		let result = value.slice(0, value.length - 27)
		if (value[value.length-27] !== '0') result += '.' + value[value.length-27]
		return result + ' KS'
	}
	let result = value.slice(0, value.length - 30)
	if (value[value.length-30] !== '0') result += '.' + value[value.length-30]
	return result + ' MS'
}

export const convertPricePerBlock = (value: string) => {
	if (value.length > 12) {
		value = value.slice(0, value.length - 12) + '.' + value.slice(value.length - 12)
	} else {
		value = '0.' + '0'.repeat(12 - value.length) + value
	}
	let result = parseFloat(value) * 144 * 30
	if (result < 1e-12) return (result * 1e24).toFixed(0) + ' H/TB/month'
	if (result < 1e-9) return (result * 1e24).toFixed(0) + ' pS/TB/month'
	if (result < 1e-6) return (result * 1e24).toFixed(0) + ' uS/TB/month'
	if (result < 1e-3) return (result * 1e24).toFixed(0) + ' mS/TB/month'
	if (result < 1) return result.toFixed(1) + ' SC/TB/month'
	if (result < 1e3) return result.toFixed(0) + ' SC/TB/month'
	if (result < 1e6) return (result / 1e3).toFixed(1) + ' KS/TB/month'
	return (result / 1e6).toFixed(1) + ' MS/TB/month'
}

export const convertPriceRaw = (value: string) => {
	if (value.length > 12) {
		value = value.slice(0, value.length - 12) + '.' + value.slice(value.length - 12)
	} else {
		value = '0.' + '0'.repeat(12 - value.length) + value
	}
	return Number.parseFloat(value)
}
