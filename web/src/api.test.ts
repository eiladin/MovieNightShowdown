import { describe, expect, it } from 'vitest'
import { buildPreviewParams } from './api'

describe('buildPreviewParams', () => {
    it('emits no params for an empty selection', () => {
        expect(buildPreviewParams({}).toString()).toBe('')
    })

    it('repeats a param per value for the list filters', () => {
        const p = buildPreviewParams({
            genres: ['Action', 'Sci-Fi'],
            officialRatings: ['PG', 'PG-13'],
            sources: ['jellyfin', 'netflix'],
        })
        expect(p.getAll('genres')).toEqual(['Action', 'Sci-Fi'])
        expect(p.getAll('officialRatings')).toEqual(['PG', 'PG-13'])
        expect(p.getAll('sources')).toEqual(['jellyfin', 'netflix'])
    })

    it('encodes values that are not URL-safe', () => {
        const p = buildPreviewParams({ genres: ['Sci-Fi & Fantasy'] })
        expect(p.toString()).toBe('genres=Sci-Fi+%26+Fantasy')
        expect(p.getAll('genres')).toEqual(['Sci-Fi & Fantasy'])
    })

    it('sets the numeric filters as strings', () => {
        const p = buildPreviewParams({ yearMin: 1980, yearMax: 1989, ratingMin: 7.5 })
        expect(p.get('yearMin')).toBe('1980')
        expect(p.get('yearMax')).toBe('1989')
        expect(p.get('ratingMin')).toBe('7.5')
    })

    // Zero is falsy, so it is omitted rather than sent. That matches
    // server.ParseFilters, which treats a missing bound as unbounded and would
    // read an explicit 0 the same way.
    it('omits zero numeric filters', () => {
        const p = buildPreviewParams({ yearMin: 0, yearMax: 0, ratingMin: 0 })
        expect(p.has('yearMin')).toBe(false)
        expect(p.has('yearMax')).toBe(false)
        expect(p.has('ratingMin')).toBe(false)
    })

    it('sends unwatched only when true', () => {
        expect(buildPreviewParams({ unwatched: true }).get('unwatched')).toBe('true')
        expect(buildPreviewParams({ unwatched: false }).has('unwatched')).toBe(false)
    })

    it('omits an empty libraryId', () => {
        expect(buildPreviewParams({ libraryId: 'abc' }).get('libraryId')).toBe('abc')
        expect(buildPreviewParams({ libraryId: '' }).has('libraryId')).toBe(false)
    })
})
