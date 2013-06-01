goog.provide('tests.start');

goog.require('goog.net.BrowserChannel');
goog.require('goog.net.ChannelDebug');
goog.require('goog.debug.Console');
goog.require('tests.Handler1');

tests.start = function() {
    goog.debug.Console.autoInstall();
    goog.debug.Console.instance.setCapturing(true);

    var clientVersion = "cv1"
    var handler = new tests.Handler1();
    var channel = new goog.net.BrowserChannel(clientVersion);
    channel.setChannelDebug(new goog.net.ChannelDebug());
    channel.setSupportsCrossDomainXhrs(true);
    channel.setAllowHostPrefix(true);
    channel.setHandler(handler);
    channel.connect('channel/test', 'channel/bind');
};

goog.exportSymbol('tests.start', tests.start);
