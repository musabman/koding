class DiscussionActivityItemView extends ActivityItemChild

  constructor:(options, data)->

    options = $.extend
      cssClass    : "activity-item discussion"
      tooltip     :
        title     : "Discussion"
        offset    : 3
        selector  : "span.type-icon"
    ,options

    super options,data

    @actionLinks = new DiscussionActivityActionsView
      delegate : @commentBox.opinionList
      cssClass : "reply-header"
    , data

    if data.repliesCount > 0
      @opinionBox = new DiscussionActivityOpinionView
        cssClass    : "activity-opinion-list comment-container"
      , data
    else
      @opinionBox = new KDCustomHTMLView
        tagName     : "div"
        cssClass    : "opinion-first-box"

      # @opinionBox.addSubView @opinionFirstLink = new KDCustomHTMLView
      #   tagName     : "a"
      #   cssClass    : "first-reply-link"
      #   attributes  :
      #     title     : "Be the first to reply"
      #     href      : "#"
      #   partial     : "Be the first to reply!"
      #   click       :->
      #     appManager.tell "Activity", "createContentDisplay", data

  viewAppended:()->
    return if @getData().constructor is bongo.api.CDiscussionActivity
    super()
    @setTemplate @pistachio()
    @template.update()

    @$("pre").addClass "prettyprint"
    prettyPrint()

    if @$("p.comment-body").height() >= 250
      @$("div.view-full-discussion").show()
    else
      @$("div.view-full-discussion").hide()


  render:->
    super()

    @$("pre").addClass "prettyprint"
    prettyPrint()

  click:(event)->
    if $(event.target).closest("[data-paths~=title],[data-paths~=body]")
      if not $(event.target).is("a.action-link, a.count, .like-view")
        appManager.tell "Activity", "createContentDisplay", @getData()

  applyTextExpansions:(str = "")->
    str = @utils.expandUsernames @utils.applyMarkdown str

    if str.length > 500
      visiblePart = str.substr 0, 500
      # this breaks the markdown sanitizer
      # morePart = "<span class='more'><a href='#' class='more-link'>show more...</a>#{str.substr 501}<a href='#' class='less-link'>...show less</a></span>"
      str = visiblePart  + " ..." #+ morePart

    return str

  pistachio:->
    """
  <div class="activity-discussion-container">
    <span class="avatar">{{> @avatar}}</span>
    <div class='activity-item-right-col'>
      {{> @settingsButton}}
      <h3 class='hidden'></h3>
      <p class="comment-title">{{@applyTextExpansions #(title)}}</p>
      <p class="comment-body has-markdown">{{@applyTextExpansions #(body)}}</p>
      <div class="view-full-discussion">
        <a href="#">View the full Discussion</a>
      </div>
      <footer class='clearfix'>
        <div class='type-and-time'>
          <span class='type-icon'></span> by {{> @author}}
          <time>{{$.timeago #(meta.createdAt)}}</time>
          {{> @tags}}
        </div>
        {{> @actionLinks}}
      </footer>
      {{> @opinionBox}}
    </div>
  </div>
    """

